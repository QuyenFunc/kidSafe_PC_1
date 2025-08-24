package main

import (
	"bufio"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// Windows Service constants
const (
	ServiceName        = "ParentalControlService"
	ServiceDisplayName = "Parental Control DNS Service"
	ServiceDescription = "DNS filtering service for parental control"
)

// SSE Client represents a connected SSE client
type SSEClient struct {
	id       string
	channel  chan string
	clientIP string
}

// Core Service struct
type CoreService struct {
	db              *sql.DB
	httpServer      *http.Server
	hostsManager    *HostsManager
	firebaseService *FirebaseService
	authService     *AuthService
	blocklist       sync.Map
	whitelist       sync.Map
	profiles        sync.Map
	config          *Config
	// SSE support for real-time updates
	sseClients map[string]*SSEClient
	sseMutex   sync.RWMutex
}

// Configuration struct
type Config struct {
	APIPort      string `json:"api_port"`
	LogLevel     string `json:"log_level"`
	DatabasePath string `json:"database_path"`
}

// Data structures
type BlockRule struct {
	ID        int    `json:"id"`
	Domain    string `json:"domain"`
	Category  string `json:"category"`
	ProfileID int    `json:"profile_id"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
	IsActive  bool   `json:"is_active"`
}

type WhitelistRule struct {
	ID        int    `json:"id"`
	Domain    string `json:"domain"`
	ProfileID int    `json:"profile_id"`
	CreatedAt string `json:"created_at"`
}

type DNSLog struct {
	ID        int    `json:"id"`
	Domain    string `json:"domain"`
	ClientIP  string `json:"client_ip"`
	QueryType string `json:"query_type"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	ProfileID int    `json:"profile_id"`
}

type Profile struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at"`
}

// Windows Service struct
type parentalControlService struct {
	coreService *CoreService
}

var instanceMutex *syscall.Handle

// Main function với service handling
func main() {
	if runtime.GOOS == "windows" {
		// Set console to UTF-8
		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
		setConsoleOutputCP.Call(65001) // UTF-8 codepage
	}
	// Check for service installation flags
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "--install":
			err := installService()
			if err != nil {
				log.Fatalf("Failed to install service: %v", err)
			}
			return
		case "--uninstall":
			err := uninstallService()
			if err != nil {
				log.Fatalf("Failed to uninstall service: %v", err)
			}
			return
		case "--start":
			err := startService()
			if err != nil {
				log.Fatalf("Failed to start service: %v", err)
			}
			return
		case "--ui":
			// Force open UI after starting service
			os.Setenv("KIDSAFE_OPEN_UI", "true")
		case "--no-ui":
			// Force console mode only
			os.Setenv("KIDSAFE_OPEN_UI", "false")
		case "--help", "-h":
			showUsage()
			return
		}
	}

	// Check if running as service
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to determine if running as service: %v", err)
	}

	if isService {
		runService()
	} else {
		runConsole()
	}
}

func runConsole() {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🚨 PANIC RECOVERED: %v", r)
			fmt.Println("Service crashed! Press Enter to exit...")
			fmt.Scanln()
			os.Exit(1)
		}
	}()

	if !isRunningAsAdmin() {
		log.Println("ERROR: This application requires Administrator privileges")
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
		os.Exit(1)
	}

	log.Println("Running with Administrator privileges ✓")

	config := &Config{
		APIPort:      "8081",
		LogLevel:     "INFO",
		DatabasePath: "./data/parental_control.db",
	}

	service, err := NewCoreService(config)
	if err != nil {
		log.Fatal("Failed to create service:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start API server
	log.Println("Starting API server...")
	if err := service.StartAPIServer(ctx); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}

	// Verify API server
	for i := 0; i < 5; i++ {
		time.Sleep(500 * time.Millisecond)
		if service.isAPIServerReady() {
			break
		}
		if i == 4 {
			log.Fatal("API server not responding")
		}
	}
	log.Println("API server confirmed ready ✓")

	// Load existing rules into hosts file
	log.Println("Applying existing block rules to hosts file...")
	if err := service.syncRulesToHosts(); err != nil {
		log.Printf("Warning: Failed to sync rules to hosts: %v", err)
	}

	// Start Firebase service if available
	if service.firebaseService != nil {
		if err := service.firebaseService.Start(); err != nil {
			log.Printf("Warning: Failed to start Firebase service: %v", err)
		} else {
			log.Println("🔥 Firebase realtime sync started")
		}
	}

	log.Println("✅ KidSafe PC started successfully using hosts-based blocking")
	log.Printf("📡 API Server: http://localhost:%s", config.APIPort)
	log.Printf("📊 Blocking %d domains", len(service.hostsManager.GetBlockedDomains()))

	// Don't auto-open browser - Electron app will handle UI

	// Keep service running - don't exit after initialization
	log.Println("🎯 Service ready - entering main loop...")

	// Wait for shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block here until shutdown signal
	sig := <-sigChan
	log.Printf("📡 Received signal: %v - shutting down...", sig)
	service.Shutdown()
}

// Add this new method to CoreService
func (s *CoreService) isAPIServerReady() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + s.config.APIPort + "/api/v1/stats")
	if err != nil {
		log.Printf("API readiness check failed: %v", err)
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// SIMPLIFIED admin check
func isRunningAsAdmin() bool {
	if runtime.GOOS != "windows" {
		return os.Geteuid() == 0
	}

	// Simple Windows admin check
	cmd := exec.Command("net", "session")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	return err == nil
}

var lockFile *os.File

func acquireInstanceLock() bool {
	lockPath := filepath.Join(os.TempDir(), "parental_control_service.lock")

	var err error
	lockFile, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		// Lock file exists, another instance is running
		return false
	}

	// Write PID to lock file
	fmt.Fprintf(lockFile, "%d", os.Getpid())
	return true
}

func releaseInstanceLock() {
	if lockFile != nil {
		lockFile.Close()
		lockPath := filepath.Join(os.TempDir(), "parental_control_service.lock")
		os.Remove(lockPath)
	}
}

// Service functions remain the same...
func runService() {
	elog, err := eventlog.Open(ServiceName)
	if err != nil {
		return
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", ServiceName))
	run := svc.Run
	err = run(ServiceName, &parentalControlService{})
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", ServiceName, err))
		return
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", ServiceName))
}

func (m *parentalControlService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	config := &Config{
		APIPort:      "8081",
		LogLevel:     "INFO",
		DatabasePath: "C:\\ProgramData\\ParentalControl\\parental_control.db",
	}

	coreService, err := NewCoreService(config)
	if err != nil {
		return true, 1
	}
	m.coreService = coreService

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start API server synchronously to ensure availability
	if err := coreService.StartAPIServer(ctx); err != nil {
		log.Printf("Failed to start API server in service mode: %v", err)
		return true, 1
	}

	// Sync existing rules to hosts file
	if err := coreService.syncRulesToHosts(); err != nil {
		log.Printf("Warning: Failed to sync rules to hosts in service mode: %v", err)
	}

	// Start Firebase service if available
	if coreService.firebaseService != nil {
		if err := coreService.firebaseService.Start(); err != nil {
			log.Printf("Warning: Failed to start Firebase service: %v", err)
		} else {
			log.Println("🔥 Firebase realtime sync started")
		}
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			goto cleanup
		default:
			log.Printf("unexpected service control request #%d", c.Cmd)
		}
	}

cleanup:

	changes <- svc.Status{State: svc.StopPending}
	coreService.Shutdown()
	return
}

func NewCoreService(config *Config) (*CoreService, error) {
	// Initialize database
	os.MkdirAll(filepath.Dir(config.DatabasePath), 0755)
	db, err := sql.Open("sqlite3", config.DatabasePath)
	if err != nil {
		return nil, err
	}

	// Initialize hosts manager
	hostsManager := NewHostsManager()
	if err := hostsManager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize hosts manager: %v", err)
	}

	service := &CoreService{
		db:           db,
		config:       config,
		hostsManager: hostsManager,
		sseClients:   make(map[string]*SSEClient),
	}

	// Initialize database tables
	if err := service.initDB(); err != nil {
		return nil, err
	}

	// Load rules into memory
	if err := service.loadRules(); err != nil {
		return nil, err
	}

	// Load profiles into memory
	if err := service.loadProfiles(); err != nil {
		return nil, err
	}

	// Initialize Auth service with real Firebase Auth
	var userUID string
	var userEmail string

	// Check if running via Electron (skip console auth prompts)
	electronModeEnv := os.Getenv("KIDSAFE_ELECTRON_MODE")
	isElectronMode := electronModeEnv == "true"
	log.Printf("[ENV] KIDSAFE_ELECTRON_MODE=%s, isElectronMode=%v", electronModeEnv, isElectronMode)

	// Try real Firebase Auth first (allow in Electron mode for UI login)
	useRealAuth := os.Getenv("KIDSAFE_USE_REAL_AUTH") != "false" // Allow Firebase auth

	// Debug environment variables
	log.Printf("[ENV] KIDSAFE_ELECTRON_MODE=%s, KIDSAFE_USE_REAL_AUTH=%s",
		os.Getenv("KIDSAFE_ELECTRON_MODE"), os.Getenv("KIDSAFE_USE_REAL_AUTH"))
	log.Printf("[ENV] Variables: isElectronMode=%v, useRealAuth=%v", isElectronMode, useRealAuth)

	if isElectronMode {
		log.Println("[AUTH] Electron mode - All authentication will be handled via UI")
	}

	// Display startup banner
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🛡️  KIDSAFE PC PARENTAL CONTROL SERVICE")
	fmt.Println(strings.Repeat("=", 70))

	if useRealAuth {
		if !isElectronMode {
			log.Println("🔐 Checking Firebase Authentication...")
		} else {
			log.Println("🖥️ Electron mode - Firebase auth available via API endpoints")
		}

		realAuth, err := NewRealFirebaseAuth()
		if err != nil {
			log.Printf("⚠️ Failed to initialize Firebase Auth: %v", err)
			log.Println("💡 Tip: Make sure firebase-config.json contains your Web API key")
			if !isElectronMode {
				// Only fallback if not in Electron mode (Electron handles auth via API)
				log.Println("🔄 Falling back to LocalAuth compatibility mode...")
				useRealAuth = false
			} else {
				log.Println("🖥️ Electron mode - Firebase auth will be handled via API endpoints")
			}
		} else {
			// Check if already authenticated
			if realAuth.IsAuthenticated() {
				userUID = realAuth.GetUID()
				userEmail = realAuth.GetEmail()

				// Beautiful login status display
				fmt.Println("\n" + strings.Repeat("✨", 35))
				fmt.Printf("✅ ALREADY LOGGED IN\n")
				fmt.Printf("👤 User: %s\n", userEmail)
				fmt.Printf("🆔 UID: %s\n", userUID)
				fmt.Printf("🕒 Session Active Since Startup\n")
				fmt.Println(strings.Repeat("✨", 35))
			} else {
				// Try to load saved token from firebase_auth_token.json
				if err := realAuth.loadSavedToken(); err == nil {
					if realAuth.IsAuthenticated() {
						userUID = realAuth.GetUID()
						userEmail = realAuth.GetEmail()

						// Beautiful restored login status display
						fmt.Println("\n" + strings.Repeat("🔄", 35))
						fmt.Printf("✅ SESSION RESTORED FROM TOKEN\n")
						fmt.Printf("👤 User: %s\n", userEmail)
						fmt.Printf("🆔 UID: %s\n", userUID)
						fmt.Printf("🕒 Token Restored Successfully\n")
						fmt.Println(strings.Repeat("🔄", 35))
						log.Println("🔥 Firebase session restored from saved token")
					} else {
						log.Println("⚠️ Saved Firebase token is invalid or expired")
					}
				} else {
					log.Printf("⚠️ No saved Firebase token found: %v", err)
				}
				// Try to get credentials from user_credentials.json (from Electron UI)
				credPaths := []string{
					"user_credentials.json",
					filepath.Join("..", "ui-admin", "user_credentials.json"),
				}

				for _, path := range credPaths {
					if data, err := os.ReadFile(path); err == nil {
						var creds struct {
							Email    string `json:"email"`
							Password string `json:"password"` // Only if saved
						}
						if json.Unmarshal(data, &creds) == nil && creds.Email != "" {
							// Prompt for password if not saved
							password := creds.Password
							if password == "" && !isElectronMode {
								fmt.Printf("🔐 Password for %s: ", creds.Email)
								fmt.Scanln(&password)
							}

							if err := realAuth.Login(creds.Email, password); err != nil {
								log.Printf("⚠️ Firebase login failed: %v", err)
							} else {
								userUID = realAuth.GetUID()
								userEmail = realAuth.GetEmail()

								// Success message
								fmt.Println("\n" + strings.Repeat("🎉", 35))
								fmt.Printf("✅ LOGIN SUCCESSFUL!\n")
								fmt.Printf("👤 Welcome: %s\n", userEmail)
								fmt.Printf("🆔 UID: %s\n", userUID)
								fmt.Println(strings.Repeat("🎉", 35))
								break
							}
						}
					}
				}

				// If still not authenticated, skip console prompts in Electron mode
				if userUID == "" && !isElectronMode {
					fmt.Println("\n" + strings.Repeat("=", 60))
					fmt.Println("🔐 KIDSAFE PC - ĐĂNG NHẬP FIREBASE")
					fmt.Println(strings.Repeat("=", 60))
					fmt.Println("Vui lòng đăng nhập với tài khoản đã dùng trên Android")
					fmt.Println()

					reader := bufio.NewReader(os.Stdin)
					fmt.Print("📧 Email: ")
					email, _ := reader.ReadString('\n')
					email = strings.TrimSpace(email)

					fmt.Print("🔑 Password: ")
					password, _ := reader.ReadString('\n')
					password = strings.TrimSpace(password)

					if err := realAuth.Login(email, password); err != nil {
						fmt.Printf("❌ Login failed: %v\n", err)
						fmt.Println("📍 Continuing with local-only mode...")
					} else {
						userUID = realAuth.GetUID()
						userEmail = realAuth.GetEmail()

						// Success message
						fmt.Println("\n" + strings.Repeat("🎉", 35))
						fmt.Printf("✅ LOGIN SUCCESSFUL!\n")
						fmt.Printf("👤 Welcome: %s\n", userEmail)
						fmt.Printf("🆔 UID: %s\n", userUID)
						fmt.Println(strings.Repeat("🎉", 35))
					}
				} else if isElectronMode {
					log.Println("[AUTH] Electron mode - Firebase login will be handled via UI")
				}
			}
		}
	}

	// If Real Auth failed, fallback to LocalAuth compatibility (UI-only mode)
	if !useRealAuth && userUID == "" {
		if isElectronMode {
			log.Println("🔵 Electron mode - All authentication will be handled via UI")
		} else {
			log.Println("🔄 Using LocalAuth compatibility mode...")
			log.Println("🔵 Authentication will be handled via UI")
		}
	}

	// If we have a valid Firebase UID, use it for Firebase Realtime Database
	if userUID != "" {
		log.Printf("🔥 Initializing Firebase service with UID: %s", userUID)
		log.Printf("📧 User email for Firebase service: %s", userEmail)

		// Calculate LocalAuth UID for compatibility
		localAuthUID := generateLocalAuthUID(userEmail)
		log.Printf("🔄 LocalAuth UID calculated: %s", localAuthUID)

		// Initialize Firebase service with the real UID and email
		firebaseService, err := SetupFirebaseServiceWithEmail(userUID, userEmail, hostsManager, service.db, service)
		if err != nil {
			log.Printf("⚠️ Firebase service initialization failed: %v", err)
			log.Println("📍 Continuing with local-only mode...")
		} else {
			service.firebaseService = firebaseService
			log.Println("🔥 Firebase service initialized successfully")
			log.Printf("📡 Listening at Firebase path: kidsafe/families/%s/blockedUrls", userUID)
			if userEmail != "" {
				localAuthUID := generateLocalAuthUID(userEmail)
				log.Printf("📡 Also checking LocalAuth path: kidsafe/families/%s/blockedUrls", localAuthUID)
			}
		}
	} else {
		log.Println("[AUTH] No Firebase authentication - running in local-only mode")
		log.Println("[AUTH] URLs blocked from Android app will NOT sync")
	}

	return service, nil
}

// generateLocalAuthUID creates the same UID as Android LocalAuthService
func generateLocalAuthUID(email string) string {
	// Match Android LocalAuthService: UID = "user_" + first 16 hex of MD5(email)
	sum := md5.Sum([]byte(email))
	hex := fmt.Sprintf("%x", sum)
	if len(hex) > 16 {
		hex = hex[:16]
	}
	return "user_" + hex
}

// discoverFirebaseCredentialsPath tries multiple locations for service account json
func discoverFirebaseCredentialsPath() string {
	// 1) Env variable override
	if env := os.Getenv("KIDSAFE_CREDENTIALS"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}

	// 2) Next to current executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		p := filepath.Join(exeDir, "firebase-credentials.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 3) Current working directory
	if p := "firebase-credentials.json"; true {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 4) ui-admin folder (when running from repo)
	if p := filepath.Join("..", "ui-admin", "firebase-credentials.json"); true {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 5) core-service folder (when running from repo root)
	if p := filepath.Join("core-service", "firebase-credentials.json"); true {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// syncRulesToHosts loads all active rules and applies them to hosts file
func (s *CoreService) syncRulesToHosts() error {
	rows, err := s.db.Query("SELECT domain FROM block_rules WHERE is_active = 1")
	if err != nil {
		return err
	}
	defer rows.Close()

	uniq := make(map[string]struct{})
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			continue
		}
		nd := normalizeDomain(domain)
		if nd != "" {
			uniq[nd] = struct{}{}
		}
	}

	var domains []string
	for d := range uniq {
		domains = append(domains, d)
	}

	return s.hostsManager.UpdateBlockedDomains(domains)
}

// normalizeDomain converts raw input like "https://www.example.com/path" to "example.com"
func normalizeDomain(raw string) string {
	r := strings.TrimSpace(strings.ToLower(raw))
	if r == "" {
		return ""
	}
	// Prepend scheme if missing so url.Parse works better
	if !strings.HasPrefix(r, "http://") && !strings.HasPrefix(r, "https://") {
		r = "http://" + r
	}
	u, err := url.Parse(r)
	if err != nil || u.Host == "" {
		// Fallback: strip protocol manually
		r = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(raw), "http://"), "https://")
		r = strings.SplitN(r, "/", 2)[0]
	} else {
		r = u.Host
	}
	// Remove port if any
	host, _, err := net.SplitHostPort(r)
	if err == nil && host != "" {
		r = host
	}
	// Remove www.
	r = strings.TrimPrefix(r, "www.")
	// Keep only valid hostname characters
	r = strings.Split(r, "?")[0]
	r = strings.Split(r, "#")[0]
	r = strings.TrimSpace(r)
	if r == "" {
		return ""
	}
	return r
}

// Simple system preparation - no DNS/firewall modifications
func (s *CoreService) prepareSystem() error {
	log.Println("Preparing system for hosts-based blocking...")

	// No DNS server configuration needed
	// No firewall modifications needed
	// Just ensure hosts file is writable

	log.Println("System preparation completed - using hosts file approach")
	return nil
}

// Database initialization
func (s *CoreService) initDB() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			is_active BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS block_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL,
			category TEXT,
			profile_id INTEGER DEFAULT 1,
			reason TEXT,
			is_active BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (profile_id) REFERENCES profiles(id)
		)`,
		`CREATE TABLE IF NOT EXISTS dns_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL,
			client_ip TEXT,
			query_type TEXT,
			action TEXT,
			profile_id INTEGER DEFAULT 1,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (profile_id) REFERENCES profiles(id)
		)`,
		`CREATE TABLE IF NOT EXISTS whitelist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL,
			profile_id INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT OR IGNORE INTO profiles (id, name, description) VALUES (1, 'Default', 'Default profile')`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

func (s *CoreService) loadRules() error {
	// Load blocklist
	rows, err := s.db.Query("SELECT domain, category FROM block_rules WHERE is_active = 1")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var domain, category string
		if err := rows.Scan(&domain, &category); err != nil {
			continue
		}
		s.blocklist.Store(strings.ToLower(domain), category)
	}

	// Load whitelist
	rows, err = s.db.Query("SELECT domain FROM whitelist")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			continue
		}
		s.whitelist.Store(strings.ToLower(domain), true)
	}

	log.Println("Block/white lists loaded into memory.")
	return nil
}

func (s *CoreService) loadProfiles() error {
	rows, err := s.db.Query("SELECT id, name, is_active FROM profiles")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.IsActive); err != nil {
			log.Printf("Warning: could not scan profile row: %v", err)
			continue
		}
		s.profiles.Store(p.ID, p)
	}

	log.Println("Profiles loaded into memory.")
	return nil
}

// DNS Server methods removed - using hosts file approach

func (s *CoreService) isBlocked(domain string) (string, bool) {
	domain = strings.ToLower(domain)
	// Exact match
	if category, exists := s.blocklist.Load(domain); exists {
		return category.(string), true
	}

	// Check parent domains
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if category, exists := s.blocklist.Load(parentDomain); exists {
			return category.(string), true
		}
	}

	return "", false
}

// DNS logging removed - using hosts file approach

// IMPROVED API Server
func (s *CoreService) StartAPIServer(ctx context.Context) error {
	router := mux.NewRouter()

	// CORS middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// API endpoints
	api := router.PathPrefix("/api/v1").Subrouter()

	// Block rules
	api.HandleFunc("/rules", s.handleGetRules).Methods("GET")
	api.HandleFunc("/rules", s.handleAddRule).Methods("POST")
	api.HandleFunc("/rules/{id}", s.handleDeleteRule).Methods("DELETE")

	// Whitelist rules
	api.HandleFunc("/whitelist", s.handleGetWhitelist).Methods("GET")
	api.HandleFunc("/whitelist", s.handleAddWhitelistRule).Methods("POST")
	api.HandleFunc("/whitelist/{id}", s.handleDeleteWhitelistRule).Methods("DELETE")

	// DNS logs
	api.HandleFunc("/logs", s.handleGetLogs).Methods("GET")

	// Profiles
	api.HandleFunc("/profiles", s.handleGetProfiles).Methods("GET")
	api.HandleFunc("/profiles", s.handleAddProfile).Methods("POST")

	// AI suggestions
	api.HandleFunc("/ai/suggest", s.handleAISuggestion).Methods("POST")

	// Stats
	api.HandleFunc("/stats", s.handleGetStats).Methods("GET")

	// System status endpoints
	api.HandleFunc("/system/status", s.handleSystemStatus).Methods("GET")
	api.HandleFunc("/system/configure", s.handleSystemConfigure).Methods("POST")
	api.HandleFunc("/system/restore", s.handleSystemRestore).Methods("POST")

	// Status endpoint
	api.HandleFunc("/status", s.handleStatus).Methods("GET")

	// Auth endpoints
	api.HandleFunc("/auth/status", s.handleAuthStatus).Methods("GET")
	api.HandleFunc("/auth/login", s.handleAuthLogin).Methods("POST")

	// Manual sync endpoint (temporary workaround)
	api.HandleFunc("/sync/firebase", s.handleManualFirebaseSync).Methods("POST")

	// Strict mode removed - keeping simple hosts-only approach

	// Debug endpoints
	api.HandleFunc("/system/verify-hosts", s.handleVerifyHosts).Methods("GET")
	api.HandleFunc("/system/test-blocking/{domain}", s.handleTestBlocking).Methods("GET")

	// Firebase endpoints - moved under /api/v1 prefix
	api.HandleFunc("/firebase/force-sync", s.handleFirebaseForceSync).Methods("POST")
	api.HandleFunc("/firebase/status", s.handleFirebaseStatus).Methods("GET")

	// Real-time updates endpoint using Server-Sent Events
	api.HandleFunc("/events/rules", s.handleRulesSSE).Methods("GET")

	server := &http.Server{
		Addr:    "127.0.0.1:" + s.config.APIPort,
		Handler: router,
	}

	s.httpServer = server
	log.Printf("API server starting on %s", server.Addr)

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	// Test API server
	time.Sleep(500 * time.Millisecond)
	resp, err := http.Get("http://" + server.Addr + "/api/v1/stats")
	if err != nil {
		return fmt.Errorf("API server failed to start on %s: %v", server.Addr, err)
	}
	resp.Body.Close()

	log.Printf("API server successfully started on %s", server.Addr)

	// Hosts file approach - no additional policies needed

	return nil
}

// DoH blocking removed - using simple hosts file approach only

// Service management functions - Keep existing
func installService() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", ServiceName)
	}

	config := mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: ServiceDisplayName,
		Description: ServiceDescription,
	}

	s, err = m.CreateService(ServiceName, exePath, config)
	if err != nil {
		return err
	}
	defer s.Close()

	eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	log.Printf("Service %s installed successfully", ServiceName)
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", ServiceName)
	}
	defer s.Close()

	// Stop service if running
	status, err := s.Query()
	if err == nil && status.State == svc.Running {
		_, err = s.Control(svc.Stop)
		if err != nil {
			log.Printf("Warning: failed to stop service: %v", err)
		}
	}

	err = s.Delete()
	if err != nil {
		return err
	}

	eventlog.Remove(ServiceName)
	log.Printf("Service %s uninstalled successfully", ServiceName)
	return nil
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}

	log.Printf("Service %s started successfully", ServiceName)
	return nil
}

// All API handlers remain the same - keeping existing implementation...
// [Include all the existing API handler functions here - they remain unchanged]

func (s *CoreService) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]interface{})

	// Count total rules
	var totalRules int
	s.db.QueryRow("SELECT COUNT(*) FROM block_rules WHERE is_active = 1").Scan(&totalRules)
	stats["total_rules"] = totalRules

	// Count blocked requests today
	var blockedToday int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM dns_logs 
		WHERE action = 'blocked' AND date(timestamp) = date('now')`).Scan(&blockedToday)
	stats["blocked_today"] = blockedToday

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// System handlers
func (s *CoreService) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	// Check if hosts file is accessible and how many domains are blocked
	blockedDomains := s.hostsManager.GetBlockedDomains()

	response := map[string]interface{}{
		"hosts_accessible": true, // Will be false if we can't write to hosts
		"blocked_domains":  len(blockedDomains),
		"method":           "hosts_file",
		"overall_status":   true, // Always true for hosts-based approach
	}

	// Add Firebase status if available
	if s.firebaseService != nil {
		firebaseStats := s.firebaseService.GetStats()
		response["firebase_connected"] = firebaseStats["is_listening"]
		response["firebase_user_uid"] = firebaseStats["family_id"]
		response["firebase_sync_count"] = firebaseStats["blocked_count"]
		response["firebase_status"] = firebaseStats["status"]
	} else {
		response["firebase_connected"] = false
		response["firebase_status"] = "not_configured"
	}

	// Add Auth status if available
	if s.authService != nil && s.authService.IsAuthenticated() {
		userInfo := s.authService.GetUserInfo()
		response["auth_status"] = "authenticated"
		response["user_email"] = userInfo.Email
		response["user_uid"] = userInfo.UID
		response["login_time"] = userInfo.LoginTime
	} else {
		response["auth_status"] = "not_authenticated"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Simple status endpoint for health check
func (s *CoreService) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"service":   "kidsafe-core",
		"timestamp": time.Now().Unix(),
	})
}

// Beautiful Auth Status API
func (s *CoreService) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"service":   "KidSafe PC",
	}

	// Check if authenticated
	if s.authService != nil && s.authService.IsAuthenticated() {
		userInfo := s.authService.GetUserInfo()
		response["authenticated"] = true
		response["user"] = map[string]interface{}{
			"email":        userInfo.Email,
			"uid":          userInfo.UID,
			"login_time":   userInfo.LoginTime,
			"display_name": userInfo.Email, // Use email as display name
		}
		response["status"] = "🟢 LOGGED IN"
		response["message"] = fmt.Sprintf("Welcome back, %s!", userInfo.Email)
	} else {
		response["authenticated"] = false
		response["user"] = nil
		response["status"] = "🔴 NOT LOGGED IN"
		response["message"] = "Please login to sync with Firebase"
	}

	// Add Firebase sync status
	if s.firebaseService != nil {
		firebaseStats := s.firebaseService.GetStats()
		response["firebase"] = map[string]interface{}{
			"connected":  firebaseStats["is_listening"],
			"sync_count": firebaseStats["blocked_count"],
			"status":     firebaseStats["status"],
		}
	} else {
		response["firebase"] = map[string]interface{}{
			"connected":  false,
			"sync_count": 0,
			"status":     "not_configured",
		}
	}

	json.NewEncoder(w).Encode(response)
}

// Strict mode handlers removed - simplified hosts-only approach

func (s *CoreService) handleSystemConfigure(w http.ResponseWriter, r *http.Request) {
	// Sync all current rules to hosts file
	err := s.syncRulesToHosts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Rules applied to hosts file"})
}

func (s *CoreService) handleSystemRestore(w http.ResponseWriter, r *http.Request) {
	err := s.hostsManager.RestoreOriginal()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Original hosts file restored"})
}

// API handlers - Add basic implementations
func (s *CoreService) handleGetRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id, domain, category, profile_id, reason, created_at, is_active FROM block_rules ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rules []BlockRule
	for rows.Next() {
		var rule BlockRule
		err := rows.Scan(&rule.ID, &rule.Domain, &rule.Category, &rule.ProfileID, &rule.Reason, &rule.CreatedAt, &rule.IsActive)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func (s *CoreService) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var rule BlockRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nd := normalizeDomain(rule.Domain)
	if nd == "" {
		http.Error(w, "Invalid domain", http.StatusBadRequest)
		return
	}

	_, err := s.db.Exec("INSERT INTO block_rules (domain, category, profile_id, reason) VALUES (?, ?, ?, ?)",
		nd, rule.Category, rule.ProfileID, rule.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add to hosts file immediately
	if err := s.hostsManager.AddBlockedDomain(nd); err != nil {
		log.Printf("Warning: Failed to add domain to hosts file: %v", err)
	}

	s.blocklist.Store(strings.ToLower(nd), rule.Category)

	// Broadcast update to SSE clients
	go s.broadcastRulesUpdate()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *CoreService) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var domain string
	err := s.db.QueryRow("SELECT domain FROM block_rules WHERE id = ?", id).Scan(&domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	_, err = s.db.Exec("DELETE FROM block_rules WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove from hosts file immediately
	if err := s.hostsManager.RemoveBlockedDomain(normalizeDomain(domain)); err != nil {
		log.Printf("Warning: Failed to remove domain from hosts file: %v", err)
	}

	s.blocklist.Delete(strings.ToLower(normalizeDomain(domain)))

	// Broadcast update to SSE clients
	go s.broadcastRulesUpdate()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *CoreService) handleGetWhitelist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]WhitelistRule{})
}

func (s *CoreService) handleAddWhitelistRule(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *CoreService) handleDeleteWhitelistRule(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *CoreService) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	rows, err := s.db.Query("SELECT id, domain, client_ip, query_type, action, profile_id, timestamp FROM dns_logs ORDER BY timestamp DESC LIMIT ?", limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []DNSLog
	for rows.Next() {
		var log DNSLog
		err := rows.Scan(&log.ID, &log.Domain, &log.ClientIP, &log.QueryType, &log.Action, &log.ProfileID, &log.Timestamp)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *CoreService) handleGetProfiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]Profile{{ID: 1, Name: "Default", IsActive: true}})
}

func (s *CoreService) handleAddProfile(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *CoreService) handleAISuggestion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"suggestions": []BlockRule{}})
}

// Debug handlers
func (s *CoreService) handleVerifyHosts(w http.ResponseWriter, r *http.Request) {
	found, err := s.hostsManager.VerifyHostsFile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "success",
		"found_domains": found,
	})
}

func (s *CoreService) handleTestBlocking(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if domain == "" {
		http.Error(w, "Domain parameter required", http.StatusBadRequest)
		return
	}

	isBlocked := s.hostsManager.TestDomainBlocking(domain)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"domain":      domain,
		"is_blocked":  isBlocked,
		"expected_ip": "127.0.0.1",
	})
}

// handleFirebaseForceSync manually triggers Firebase sync
func (s *CoreService) handleFirebaseForceSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.firebaseService == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Firebase service not available",
		})
		return
	}

	if err := s.firebaseService.ForceSync(); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Firebase sync completed successfully",
	})
}

// handleFirebaseStatus returns current Firebase status and synced URLs
func (s *CoreService) handleFirebaseStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.firebaseService == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected": false,
			"error":     "Firebase service not available",
		})
		return
	}

	stats := s.firebaseService.GetStats()
	blockedUrls := s.firebaseService.GetBlockedUrls()

	// Get Firebase domains currently in database
	var firebaseRules []map[string]interface{}
	rows, err := s.db.Query("SELECT id, domain, reason, created_at FROM block_rules WHERE category = 'firebase-sync' AND is_active = 1")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var domain, reason, createdAt string
			if err := rows.Scan(&id, &domain, &reason, &createdAt); err == nil {
				firebaseRules = append(firebaseRules, map[string]interface{}{
					"id":         id,
					"domain":     domain,
					"reason":     reason,
					"created_at": createdAt,
					"source":     "firebase",
				})
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected":      stats["is_listening"],
		"family_id":      stats["family_id"],
		"user_email":     stats["user_email"],
		"blocked_count":  stats["blocked_count"],
		"firebase_urls":  blockedUrls,
		"database_rules": firebaseRules,
		"last_updated":   stats["last_updated"],
	})
}

// DNS preparation methods removed - using hosts file approach

// Shutdown method - clean hosts file and close resources
func (s *CoreService) Shutdown() {
	log.Println("Shutting down KidSafe PC...")

	// Stop Firebase service
	if s.firebaseService != nil {
		log.Println("Stopping Firebase service...")
		if err := s.firebaseService.Stop(); err != nil {
			log.Printf("Warning: Failed to stop Firebase service: %v", err)
		}
	}

	// Stop Auth service
	if s.authService != nil {
		log.Println("Stopping Auth service...")
		s.authService.Stop()
	}

	// Restore original hosts file
	if s.hostsManager != nil {
		log.Println("Restoring original hosts file...")
		if err := s.hostsManager.Cleanup(); err != nil {
			log.Printf("Warning: Failed to cleanup hosts file: %v", err)
		}
	}

	// Stop HTTP server
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	// Close database
	if s.db != nil {
		s.db.Close()
	}

	log.Println("KidSafe PC shutdown completed")
}

// isInteractiveSession checks if this is an interactive console session
func isInteractiveSession() bool {
	// Check if we have a console window (not running as service)
	if runtime.GOOS == "windows" {
		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
		hwnd, _, _ := getConsoleWindow.Call()
		return hwnd != 0
	}
	return true
}

// openWebUI opens the web interface in default browser
func openWebUI(port string) {
	// Wait a moment for the API server to be fully ready
	time.Sleep(2 * time.Second)

	// First try to find the UI files
	uiPaths := []string{
		"../ui-admin/renderer/index.html",
		"ui-admin/renderer/index.html",
		"renderer/index.html",
	}

	var uiPath string
	for _, path := range uiPaths {
		if _, err := os.Stat(path); err == nil {
			uiPath = path
			break
		}
	}

	if uiPath == "" {
		log.Println("⚠️ UI files not found, showing status only")
		log.Printf("📡 API Server: http://localhost:%s/api/v1/system/status", port)
		log.Println("💡 To access full UI, run: npm start in ui-admin folder")
		return
	}

	// Try to open the HTML file directly
	absPath, _ := filepath.Abs(uiPath)
	url := "file:///" + strings.ReplaceAll(absPath, "\\", "/")

	log.Printf("🌐 Opening KidSafe PC UI: %s", uiPath)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("⚠️ Could not open browser automatically: %v", err)
		log.Printf("💡 Please manually open: %s", uiPath)
		log.Printf("📡 Or use API directly: http://localhost:%s", port)
	}
}

// showUsage displays help information
func showUsage() {
	fmt.Println(`
🛡️  KIDSAFE PC - PARENTAL CONTROL SERVICE
========================================

USAGE:
  kidsafe-pc.exe [OPTIONS]

OPTIONS:
  (no args)      Start service with automatic UI (default)
  --ui           Force open web UI after starting
  --no-ui        Console mode only (no UI)
  --install      Install as Windows Service
  --uninstall    Uninstall Windows Service
  --start        Start Windows Service
  --help, -h     Show this help

EXAMPLES:
  kidsafe-pc.exe              # Start with UI (recommended)
  kidsafe-pc.exe --no-ui      # Console only
  kidsafe-pc.exe --install    # Install as service

FEATURES:
  🔥 Firebase realtime sync with Android app
  🛡️ Hosts-based domain blocking
  📡 Web API on port 8081
  🌐 Beautiful web interface

For more info: https://github.com/yourrepo/kidsafe
`)
}

// Handle Firebase login from Electron app
func (s *CoreService) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var loginRequest struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginRequest); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if loginRequest.Email == "" || loginRequest.Password == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Email and password are required",
		})
		return
	}

	// Try Firebase Auth using existing auth service
	realAuth, err := NewRealFirebaseAuth()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Firebase Auth not available: " + err.Error(),
		})
		return
	}

	// Attempt login
	if err := realAuth.Login(loginRequest.Email, loginRequest.Password); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Login successful - get UID and email
	userUID := realAuth.GetUID()
	userEmail := realAuth.GetEmail()

	// Create auth service if not exists
	if s.authService == nil {
		// Try to create auth service with Firebase credentials
		credPath := discoverFirebaseCredentialsPath()
		if credPath != "" {
			authService, err := NewAuthService(credPath)
			if err == nil {
				s.authService = authService
			}
		}
	}

	// Update auth service with user info
	if s.authService != nil {
		s.authService.SetCredentialsFromFile(userUID, userEmail, time.Now().UnixMilli())
		log.Printf("[AUTH] User info updated in auth service: %s", userEmail)
	} else {
		// Create a minimal auth service just to store user info
		s.authService = &AuthService{
			userUID: userUID,
			userInfo: &UserInfo{
				UID:       userUID,
				Email:     userEmail,
				LoginTime: time.Now().UnixMilli(),
			},
		}
		log.Printf("[AUTH] Created minimal auth service for: %s", userEmail)
	}

	// Initialize Firebase service with the authenticated user
	if s.firebaseService == nil {
		firebaseService, err := SetupFirebaseServiceWithEmail(userUID, userEmail, s.hostsManager, s.db, s)
		if err != nil {
			log.Printf("⚠️ Firebase service initialization failed: %v", err)
		} else {
			s.firebaseService = firebaseService
			if err := s.firebaseService.Start(); err != nil {
				log.Printf("Warning: Failed to start Firebase service: %v", err)
			} else {
				log.Println("🔥 Firebase realtime sync started")
				localAuthUID := generateLocalAuthUID(userEmail)
				log.Printf("📡 Monitoring both Firebase UID (%s) and LocalAuth UID (%s)", userUID, localAuthUID)
			}
		}
	}

	log.Printf("✅ Electron login successful for: %s (UID: %s)", userEmail, userUID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"uid":     userUID,
		"email":   userEmail,
		"message": "Login successful",
	})
}

// handleManualFirebaseSync manually triggers Firebase sync and updates database
func (s *CoreService) handleManualFirebaseSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.firebaseService == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Firebase service not available",
		})
		return
	}

	// Get blocked URLs from Firebase service
	blockedUrls := s.firebaseService.GetBlockedUrls()
	if len(blockedUrls) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No URLs found in Firebase",
		})
		return
	}

	// Extract domains from blocked URLs
	var domains []string
	for _, blockedUrl := range blockedUrls {
		if blockedUrl != nil && blockedUrl.Status == "active" {
			// Extract domain from URL
			domain := normalizeDomain(blockedUrl.URL)
			if domain != "" {
				domains = append(domains, domain)
			}
		}
	}

	if len(domains) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No valid domains found",
		})
		return
	}

	// Update hosts file
	if err := s.hostsManager.UpdateBlockedDomains(domains); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to update hosts file: " + err.Error(),
		})
		return
	}

	// Update database - remove existing firebase-sync rules
	_, err := s.db.Exec("DELETE FROM block_rules WHERE category = 'firebase-sync'")
	if err != nil {
		log.Printf("Warning: Failed to clear existing firebase-sync rules: %v", err)
	}

	// Add new firebase-sync rules to database
	for _, domain := range domains {
		_, err := s.db.Exec(
			"INSERT INTO block_rules (domain, category, profile_id, reason, is_active) VALUES (?, ?, ?, ?, ?)",
			domain, "firebase-sync", 1, "Synced from Android app", true)
		if err != nil {
			log.Printf("Warning: Failed to insert firebase-sync rule for %s: %v", domain, err)
		}
	}

	log.Printf("📱 Manual sync completed: %d URLs synced to database", len(domains))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully synced %d URLs from Firebase", len(domains)),
		"domains": domains,
	})
}

// SSE (Server-Sent Events) implementation for real-time updates
func (s *CoreService) handleRulesSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Generate unique client ID
	clientID := fmt.Sprintf("client_%d_%s", time.Now().UnixNano(), r.RemoteAddr)

	// Create client channel
	clientChan := make(chan string, 10)

	// Create and register client
	client := &SSEClient{
		id:       clientID,
		channel:  clientChan,
		clientIP: r.RemoteAddr,
	}

	s.sseMutex.Lock()
	s.sseClients[clientID] = client
	s.sseMutex.Unlock()

	log.Printf("📡 SSE client connected: %s from %s", clientID, r.RemoteAddr)

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"message\":\"Real-time updates connected\"}\n\n")
	w.(http.Flusher).Flush()

	// Send current rules immediately
	s.sendCurrentRulesToClient(w)

	// Handle client disconnect
	defer func() {
		s.sseMutex.Lock()
		delete(s.sseClients, clientID)
		s.sseMutex.Unlock()
		close(clientChan)
		log.Printf("📡 SSE client disconnected: %s", clientID)
	}()

	// Listen for messages or client disconnect
	for {
		select {
		case message := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", message)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Send current rules to a specific SSE client
func (s *CoreService) sendCurrentRulesToClient(w http.ResponseWriter) {
	rows, err := s.db.Query("SELECT id, domain, category, profile_id, reason, created_at, is_active FROM block_rules ORDER BY created_at DESC")
	if err != nil {
		log.Printf("Error querying rules for SSE: %v", err)
		return
	}
	defer rows.Close()

	var rules []BlockRule
	for rows.Next() {
		var rule BlockRule
		err := rows.Scan(&rule.ID, &rule.Domain, &rule.Category, &rule.ProfileID, &rule.Reason, &rule.CreatedAt, &rule.IsActive)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	rulesJSON, _ := json.Marshal(map[string]interface{}{
		"type":  "rules_update",
		"rules": rules,
	})

	fmt.Fprintf(w, "data: %s\n\n", string(rulesJSON))
	w.(http.Flusher).Flush()
}

// Broadcast rules update to all connected SSE clients
func (s *CoreService) broadcastRulesUpdate() {
	s.sseMutex.RLock()
	defer s.sseMutex.RUnlock()

	if len(s.sseClients) == 0 {
		return
	}

	// Get current rules
	rows, err := s.db.Query("SELECT id, domain, category, profile_id, reason, created_at, is_active FROM block_rules ORDER BY created_at DESC")
	if err != nil {
		log.Printf("Error querying rules for broadcast: %v", err)
		return
	}
	defer rows.Close()

	var rules []BlockRule
	for rows.Next() {
		var rule BlockRule
		err := rows.Scan(&rule.ID, &rule.Domain, &rule.Category, &rule.ProfileID, &rule.Reason, &rule.CreatedAt, &rule.IsActive)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	message, _ := json.Marshal(map[string]interface{}{
		"type":  "rules_update",
		"rules": rules,
	})

	log.Printf("📡 Broadcasting rules update to %d SSE clients", len(s.sseClients))

	// Send to all clients
	for clientID, client := range s.sseClients {
		select {
		case client.channel <- string(message):
			// Message sent successfully
		default:
			// Channel is full, client might be slow - remove it
			log.Printf("⚠️ Removing slow SSE client: %s", clientID)
			delete(s.sseClients, clientID)
			close(client.channel)
		}
	}
}

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"golang.org/x/sys/windows"
	"google.golang.org/api/option"
)

// AuthService handles Firebase Authentication for PC
type AuthService struct {
	client   *auth.Client
	ctx      context.Context
	cancel   context.CancelFunc
	userUID  string
	userInfo *UserInfo
}

// UserInfo represents authenticated user information
type UserInfo struct {
	UID         string `json:"uid"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName,omitempty"`
	LoginTime   int64  `json:"loginTime"`
}

// LoginCredentials for email/password login
type LoginCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// NewAuthService creates a new Firebase Auth service
func NewAuthService(credentialsPath string) (*AuthService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize Firebase app
	opt := option.WithCredentialsFile(credentialsPath)
	config := &firebase.Config{
		ProjectID: "kidsafe-control",
	}

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error initializing Firebase app: %v", err)
	}

	// Get Auth client
	client, err := app.Auth(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error getting Auth client: %v", err)
	}

	as := &AuthService{
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}

	log.Println("Firebase Auth service initialized")
	return as, nil
}

// ShowLoginPrompt displays login UI in console
func (as *AuthService) ShowLoginPrompt() error {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🔐 KIDSAFE PC - ĐĂNG NHẬP TÀI KHOẢN")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("Để đồng bộ với ứng dụng Android, vui lòng đăng nhập")
	fmt.Println("bằng tài khoản đã sử dụng trên điện thoại.")
	fmt.Println()

	// Check if saved credentials exist
	if savedUser := as.loadSavedCredentials(); savedUser != nil {
		fmt.Printf("Tìm thấy tài khoản đã lưu: %s\n", savedUser.Email)
		fmt.Print("Sử dụng tài khoản này? (Y/n): ")

		reader := bufio.NewReader(os.Stdin)
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "" || strings.ToLower(choice) == "y" {
			as.userUID = savedUser.UID
			as.userInfo = savedUser
			fmt.Printf("✅ Đã đăng nhập với tài khoản: %s\n", savedUser.Email)
			return nil
		}
	}

	// Get credentials from user
	email, password, err := as.getCredentialsFromUser()
	if err != nil {
		return err
	}

	// Authenticate with Firebase (verify credentials)
	if err := as.verifyCredentials(email, password); err != nil {
		return fmt.Errorf("đăng nhập thất bại: %v", err)
	}

	fmt.Printf("✅ Đăng nhập thành công với tài khoản: %s\n", email)
	return nil
}

// getCredentialsFromUser prompts user for email and password
func (as *AuthService) getCredentialsFromUser() (string, string, error) {
	// Console authentication disabled in Electron mode
	return "", "", fmt.Errorf("console authentication is disabled - use UI instead")
}

// getPasswordInput gets password with hidden input (Windows)
func (as *AuthService) getPasswordInput() (string, error) {
	var password []byte
	var char [1]byte

	for {
		// Read one character at a time
		n, err := os.Stdin.Read(char[:])
		if err != nil {
			return "", err
		}

		if n == 0 {
			continue
		}

		// Check for Enter key
		if char[0] == '\r' || char[0] == '\n' {
			fmt.Println() // New line after password input
			break
		}

		// Check for Backspace
		if char[0] == '\b' || char[0] == 127 {
			if len(password) > 0 {
				password = password[:len(password)-1]
				fmt.Print("\b \b") // Erase character
			}
		} else {
			password = append(password, char[0])
			fmt.Print("*") // Show asterisk
		}
	}

	return string(password), nil
}

// verifyCredentials checks credentials with Firebase Auth REST API
func (as *AuthService) verifyCredentials(email, password string) error {
	// Basic validation first
	if !strings.Contains(email, "@") {
		return fmt.Errorf("email không hợp lệ")
	}

	if len(password) < 6 {
		return fmt.Errorf("mật khẩu phải có ít nhất 6 ký tự")
	}

	// Use Firebase Auth REST API to verify credentials
	userUID, err := as.authenticateWithFirebaseAPI(email, password)
	if err != nil {
		return fmt.Errorf("xác thực Firebase thất bại: %v", err)
	}

	// Set authenticated user info - use the real Firebase UID
	as.userUID = userUID
	as.userInfo = &UserInfo{
		UID:       userUID, // This is the real Firebase Auth UID
		Email:     email,
		LoginTime: time.Now().UnixMilli(),
	}

	// Save credentials for future use
	if err := as.saveCredentials(); err != nil {
		log.Printf("Warning: Could not save credentials: %v", err)
	}

	log.Printf("✅ Authentication successful for user: %s (UID: %s)", email, userUID)
	return nil
}

// authenticateWithFirebaseAPI verifies credentials using Firebase Auth REST API
func (as *AuthService) authenticateWithFirebaseAPI(email, password string) (string, error) {
	// Load Firebase config
	apiKey, err := as.loadFirebaseAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to load Firebase API key: %v", err)
	}

	url := fmt.Sprintf("https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s", apiKey)

	// Request payload
	payload := map[string]interface{}{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make HTTP request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Check for errors
	if resp.StatusCode != 200 {
		if errorMsg, exists := result["error"].(map[string]interface{}); exists {
			if message, ok := errorMsg["message"].(string); ok {
				return "", fmt.Errorf("Firebase Auth error: %s", message)
			}
		}
		return "", fmt.Errorf("authentication failed with status %d", resp.StatusCode)
	}

	// Extract user ID
	localId, exists := result["localId"].(string)
	if !exists || localId == "" {
		return "", fmt.Errorf("invalid response: missing localId")
	}

	log.Printf("🔐 Firebase Auth successful for email: %s", email)
	return localId, nil
}

// loadFirebaseAPIKey loads the API key from config file
func (as *AuthService) loadFirebaseAPIKey() (string, error) {
	configPaths := []string{
		"firebase-config.json",
		"../firebase-config.json",
		"../ui-admin/firebase-config.json",
	}

	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			var config struct {
				APIKey string `json:"apiKey"`
			}
			if err := json.Unmarshal(data, &config); err == nil && config.APIKey != "" {
				if config.APIKey == "YOUR_FIREBASE_WEB_API_KEY" {
					return "", fmt.Errorf("please update firebase-config.json with your actual Firebase API key")
				}
				return config.APIKey, nil
			}
		}
	}

	return "", fmt.Errorf("firebase-config.json not found or invalid - please create it with your Firebase Web API key")
}

// generateUserUID creates a consistent UID from email (fallback method)
func (as *AuthService) generateUserUID(email string) string {
	// Match Android LocalAuthService: UID = "user_" + first 16 hex of MD5(email)
	sum := md5.Sum([]byte(email))
	hex := fmt.Sprintf("%x", sum)
	if len(hex) > 16 {
		hex = hex[:16]
	}
	return "user_" + hex
}

// saveCredentials saves user info to local file
func (as *AuthService) saveCredentials() error {
	credFile := "kidsafe_user.json"

	data, err := json.MarshalIndent(as.userInfo, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(credFile, data, 0600)
}

// loadSavedCredentials loads previously saved user info
func (as *AuthService) loadSavedCredentials() *UserInfo {
	credFile := "kidsafe_user.json"

	data, err := os.ReadFile(credFile)
	if err != nil {
		return nil
	}

	var userInfo UserInfo
	if err := json.Unmarshal(data, &userInfo); err != nil {
		return nil
	}

	// Check if credentials are not too old (7 days)
	sevenDaysAgo := time.Now().AddDate(0, 0, -7).UnixMilli()
	if userInfo.LoginTime < sevenDaysAgo {
		return nil
	}

	return &userInfo
}

// GetUserUID returns current user UID
func (as *AuthService) GetUserUID() string {
	return as.userUID
}

// GetUserInfo returns current user info
func (as *AuthService) GetUserInfo() *UserInfo {
	return as.userInfo
}

// IsAuthenticated checks if user is logged in
func (as *AuthService) IsAuthenticated() bool {
	return as.userUID != "" && as.userInfo != nil
}

// SetCredentialsFromFile loads credentials from saved file
func (as *AuthService) SetCredentialsFromFile(uid, email string, loginTime int64) {
	as.userUID = uid
	as.userInfo = &UserInfo{
		UID:       uid,
		Email:     email,
		LoginTime: loginTime,
	}
	log.Printf("✅ Credentials loaded for user: %s", email)
}

// Logout clears user session
func (as *AuthService) Logout() error {
	as.userUID = ""
	as.userInfo = nil

	// Remove saved credentials
	credFile := "kidsafe_user.json"
	if err := os.Remove(credFile); err != nil && !os.IsNotExist(err) {
		return err
	}

	log.Println("User logged out successfully")
	return nil
}

// Stop stops the auth service
func (as *AuthService) Stop() {
	if as.cancel != nil {
		as.cancel()
	}
}

// ValidateToken validates a Firebase ID token (for API endpoints)
func (as *AuthService) ValidateToken(idToken string) (*auth.Token, error) {
	token, err := as.client.VerifyIDToken(as.ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %v", err)
	}
	return token, nil
}

// CreateCustomToken creates a custom token for a user (admin only)
func (as *AuthService) CreateCustomToken(uid string) (string, error) {
	token, err := as.client.CustomToken(as.ctx, uid)
	if err != nil {
		return "", fmt.Errorf("error creating custom token: %v", err)
	}
	return token, nil
}

// Helper function to enable password input mode on Windows
func enablePasswordMode() {
	handle := windows.Handle(uintptr(syscall.Stdin))
	var mode uint32
	windows.GetConsoleMode(handle, &mode)
	mode &^= windows.ENABLE_ECHO_INPUT
	windows.SetConsoleMode(handle, mode)
}

// Helper function to disable password input mode on Windows
func disablePasswordMode() {
	handle := windows.Handle(uintptr(syscall.Stdin))
	var mode uint32
	windows.GetConsoleMode(handle, &mode)
	mode |= windows.ENABLE_ECHO_INPUT
	windows.SetConsoleMode(handle, mode)
}

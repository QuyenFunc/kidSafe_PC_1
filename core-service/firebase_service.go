package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
	"google.golang.org/api/option"
)

// Firebase service configuration
type FirebaseService struct {
	app          *firebase.App
	client       *db.Client
	familyID     string
	userEmail    string // Add email field to calculate LocalAuth UID
	hostsManager *HostsManager
	database     *sql.DB      // Add database reference for syncing
	coreService  *CoreService // Add reference to core service for proper sync
	ctx          context.Context
	cancel       context.CancelFunc
	isListening  bool
	mutex        sync.Mutex
	blockedUrls  map[string]*BlockedUrl
	timeRules    map[string]*AndroidTimeRule // Track time rules from Android
}

// BlockedUrl represents a URL blocked by the parent app
type BlockedUrl struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	AddedAt int64  `json:"addedAt"`
	AddedBy string `json:"addedBy"`
	Status  string `json:"status"`
}

// AndroidTimeRule represents a time rule from Android app
type AndroidTimeRule struct {
	Active               bool   `json:"active"`
	AddedBy              string `json:"addedBy"`
	BreakDurationMinutes int    `json:"breakDurationMinutes"`
	BreakIntervalMinutes int    `json:"breakIntervalMinutes"`
	CreatedAt            int64  `json:"createdAt"`
	DailyLimitMinutes    int    `json:"dailyLimitMinutes"`
	Description          string `json:"description"`
	Name                 string `json:"name"`
	RuleType             string `json:"ruleType"`
	UpdatedAt            int64  `json:"updatedAt"`
}

// PCStatus represents the status of the PC application
type PCStatus struct {
	LastSeen       int64  `json:"lastSeen"`
	Status         string `json:"status"`
	Version        string `json:"version"`
	HostFileStatus string `json:"hostFileStatus"`
	BlockedCount   int    `json:"blockedCount"`
}

// NewFirebaseService creates a new Firebase service instance
func NewFirebaseService(credentialsPath string, userUID string, hostsManager *HostsManager, database *sql.DB, coreService *CoreService) (*FirebaseService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize Firebase app
	opt := option.WithCredentialsFile(credentialsPath)
	config := &firebase.Config{
		DatabaseURL: "https://kidsafe-control-default-rtdb.asia-southeast1.firebasedatabase.app/",
	}

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error initializing Firebase app: %v", err)
	}

	// Get database client
	client, err := app.Database(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error getting Database client: %v", err)
	}

	fs := &FirebaseService{
		app:          app,
		client:       client,
		familyID:     userUID,
		userEmail:    "", // Will be set later if available
		hostsManager: hostsManager,
		database:     database,
		coreService:  coreService,
		ctx:          ctx,
		cancel:       cancel,
		isListening:  false,
		blockedUrls:  make(map[string]*BlockedUrl),
		timeRules:    make(map[string]*AndroidTimeRule),
	}

	log.Printf("Firebase service initialized for user: %s", userUID)
	return fs, nil
}

// SetCoreService sets the core service reference (used to avoid circular dependency)
func (fs *FirebaseService) SetCoreService(coreService *CoreService) {
	fs.coreService = coreService
}

// Start begins listening for Firebase changes
func (fs *FirebaseService) Start() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if fs.isListening {
		return fmt.Errorf("Firebase service is already listening")
	}

	// Start listening for blocked URLs changes
	go fs.listenForBlockedUrls()

	// Start listening for time rules changes
	go fs.listenForTimeRules()

	// Update PC status periodically
	go fs.updatePCStatusPeriodically()

	fs.isListening = true
	log.Println("Firebase service started successfully")
	return nil
}

// Stop stops the Firebase service
func (fs *FirebaseService) Stop() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if !fs.isListening {
		return nil
	}

	fs.cancel()
	fs.isListening = false
	log.Println("Firebase service stopped")
	return nil
}

// listenForBlockedUrls uses real-time Firebase listeners for instant updates
func (fs *FirebaseService) listenForBlockedUrls() {
	// Try multiple possible paths for backward compatibility
	possiblePaths := []string{
		fmt.Sprintf("kidsafe/families/%s/blockedUrls", fs.familyID), // Correct Firebase Auth UID path
		"kidsafe/blockedUrls",                               // Legacy single path (fallback)
		fmt.Sprintf("kidsafe/blockedUrls_%s", fs.familyID),  // Alternative format
		fmt.Sprintf("families/%s/blockedUrls", fs.familyID), // Without kidsafe prefix
		fmt.Sprintf("users/%s/blockedUrls", fs.familyID),    // Alternative structure
		"blockedUrls", // Direct root path
	}

	// IMPORTANT: Also check LocalAuth UID path if we have an email
	// This is needed because Android app might be using LocalAuth instead of Firebase Auth
	if fs.userEmail != "" {
		localAuthUID := generateLocalAuthUID(fs.userEmail)
		// Only add if it's different from the Firebase UID
		if localAuthUID != fs.familyID {
			possiblePaths = append(possiblePaths, fmt.Sprintf("kidsafe/families/%s/blockedUrls", localAuthUID))
			log.Printf("🔄 Also checking LocalAuth UID path for email %s: %s", fs.userEmail, localAuthUID)
		}
	}

	log.Printf("🔥 Firebase listener started for family: %s", fs.familyID)
	log.Printf("📧 User email: %s", fs.userEmail)
	log.Printf("📡 Will check %d paths for compatibility with Android app...", len(possiblePaths))

	// Debug: Print all paths being checked
	for i, path := range possiblePaths {
		log.Printf("   Path %d: %s", i+1, path)
	}

	// Start polling all possible paths
	fs.optimizedPollingMultiplePaths(possiblePaths)
}

// optimizedPollingMultiplePaths polls multiple Firebase paths to find data
func (fs *FirebaseService) optimizedPollingMultiplePaths(paths []string) {
	pollInterval := 2 * time.Second
	maxInterval := 30 * time.Second

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	consecutiveNoChanges := 0
	var activePath string

	for {
		select {
		case <-ticker.C:
			var foundData map[string]*BlockedUrl
			var workingPath string

			// Try each path until we find data
			for _, path := range paths {
				ref := fs.client.NewRef(path)
				var urlsData map[string]*BlockedUrl

				if err := ref.Get(fs.ctx, &urlsData); err != nil {
					log.Printf("❌ Error checking path %s: %v", path, err)
					continue
				}

				// Debug: Also try different data structures
				if len(urlsData) == 0 {
					// Try as array structure
					var urlsArray []*BlockedUrl
					if err := ref.Get(fs.ctx, &urlsArray); err == nil && len(urlsArray) > 0 {
						log.Printf("🔍 Found array data at path: %s (%d URLs)", path, len(urlsArray))
						// Convert array to map
						urlsData = make(map[string]*BlockedUrl)
						for i, url := range urlsArray {
							if url != nil {
								key := fmt.Sprintf("url_%d", i)
								urlsData[key] = url
							}
						}
					} else {
						// Try as generic interface to see raw structure
						var rawData interface{}
						if err := ref.Get(fs.ctx, &rawData); err == nil && rawData != nil {
							log.Printf("🔍 Found raw data at path %s: %+v", path, rawData)
						}
					}
				}

				if len(urlsData) > 0 {
					foundData = urlsData
					workingPath = path

					// If this is a new working path, log it
					if activePath != workingPath {
						log.Printf("✅ Found data at path: %s (%d URLs)", workingPath, len(urlsData))
						activePath = workingPath

						// Debug: Print first few URLs
						count := 0
						for key, url := range urlsData {
							if count < 3 && url != nil {
								log.Printf("   Sample URL %d: %s -> %s (status: %s)", count+1, key, url.URL, url.Status)
								count++
							}
						}
					}
					break
				}
			}

			if foundData == nil {
				log.Printf("[FIREBASE] No data found in any path (checked %d paths)", len(paths))
				consecutiveNoChanges++

				if consecutiveNoChanges > 5 && pollInterval < maxInterval {
					pollInterval = time.Duration(float64(pollInterval) * 1.2)
					if pollInterval > maxInterval {
						pollInterval = maxInterval
					}
					ticker.Reset(pollInterval)
					log.Printf("[FIREBASE] Adjusting poll interval to %v", pollInterval)
				}
				continue
			}

			// Check if data changed
			fs.mutex.Lock()
			changed := len(fs.blockedUrls) != len(foundData)
			if !changed {
				// Check for additions or modifications
				for k, v := range foundData {
					if existing, ok := fs.blockedUrls[k]; !ok || existing.URL != v.URL || existing.Status != v.Status {
						changed = true
						break
					}
				}

				// Check for deletions (items that exist in fs.blockedUrls but not in foundData)
				if !changed {
					for k := range fs.blockedUrls {
						if _, exists := foundData[k]; !exists {
							changed = true
							log.Printf("🗑️ Detected deletion: %s no longer in Firebase data", k)
							break
						}
					}
				}
			}

			if changed {
				log.Printf("🔥 Firebase data changed at %s: %d URLs found", activePath, len(foundData))
				fs.blockedUrls = foundData
				if fs.blockedUrls == nil {
					fs.blockedUrls = make(map[string]*BlockedUrl)
				}
				fs.mutex.Unlock()

				// Reset to fast polling after changes
				pollInterval = 2 * time.Second
				ticker.Reset(pollInterval)
				consecutiveNoChanges = 0

				var urls []string
				for _, blockedUrl := range foundData {
					if blockedUrl != nil && blockedUrl.Status == "active" {
						cleanedUrl := fs.extractDomain(blockedUrl.URL)
						if cleanedUrl != "" {
							urls = append(urls, cleanedUrl)
						}
					}
				}

				// Force immediate hosts file update
				log.Printf("🔄 Processing %d URLs for hosts file update...", len(urls))
				for i, url := range urls {
					log.Printf("   URL %d: %s", i+1, url)
				}

				if err := fs.updateHostsFile(urls); err != nil {
					log.Printf("❌ Error updating hosts file: %v", err)
				} else {
					log.Printf("✅ Hosts file updated with %d URLs from %s", len(urls), activePath)

					// Verify hosts file was actually updated
					if fs.hostsManager != nil {
						currentBlocked := fs.hostsManager.GetBlockedDomains()
						log.Printf("✅ Hosts file now contains %d blocked domains", len(currentBlocked))
					}
				}

				// FORCE save to local database for UI display
				log.Printf("🔄 Syncing %d Firebase URLs to local database...", len(foundData))
				if err := fs.syncToLocalDatabase(foundData); err != nil {
					log.Printf("❌ Error syncing to local database: %v", err)
				} else {
					log.Printf("✅ Firebase URLs synced to local database successfully")

					// Trigger database reload to refresh UI
					log.Printf("🔄 Triggering database rules reload...")
				}

				go fs.updatePCStatus()
			} else {
				fs.mutex.Unlock()
				consecutiveNoChanges++

				if consecutiveNoChanges > 5 && pollInterval < maxInterval {
					pollInterval = time.Duration(float64(pollInterval) * 1.2)
					if pollInterval > maxInterval {
						pollInterval = maxInterval
					}
					ticker.Reset(pollInterval)
				}
			}

		case <-fs.ctx.Done():
			return
		}
	}
}

// optimizedPolling uses smart polling with exponential backoff
func (fs *FirebaseService) optimizedPolling(ref *db.Ref) {
	pollInterval := 2 * time.Second // Start with 2 seconds
	maxInterval := 30 * time.Second

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	consecutiveNoChanges := 0

	for {
		select {
		case <-ticker.C:
			var urlsData map[string]*BlockedUrl
			if err := ref.Get(fs.ctx, &urlsData); err != nil {
				log.Printf("❌ Firebase polling error: %v", err)
				// Increase interval on error
				if pollInterval < maxInterval {
					pollInterval = time.Duration(float64(pollInterval) * 1.5)
					ticker.Reset(pollInterval)
				}
				continue
			}

			fs.mutex.Lock()
			changed := len(fs.blockedUrls) != len(urlsData)
			if !changed {
				// Check for additions or modifications
				for k, v := range urlsData {
					if existing, ok := fs.blockedUrls[k]; !ok || existing.URL != v.URL || existing.Status != v.Status {
						changed = true
						break
					}
				}

				// Check for deletions (items that exist in fs.blockedUrls but not in urlsData)
				if !changed {
					for k := range fs.blockedUrls {
						if _, exists := urlsData[k]; !exists {
							changed = true
							log.Printf("🗑️ Detected deletion in optimized polling: %s no longer in Firebase data", k)
							break
						}
					}
				}
			}

			if changed {
				log.Printf("🔥 Firebase data changed: %d URLs found", len(urlsData))
				fs.blockedUrls = urlsData
				if fs.blockedUrls == nil {
					fs.blockedUrls = make(map[string]*BlockedUrl)
				}
				fs.mutex.Unlock()

				// Reset to fast polling after changes
				pollInterval = 2 * time.Second
				ticker.Reset(pollInterval)
				consecutiveNoChanges = 0

				var urls []string
				for _, blockedUrl := range urlsData {
					if blockedUrl != nil && blockedUrl.Status == "active" {
						cleanedUrl := fs.extractDomain(blockedUrl.URL)
						if cleanedUrl != "" {
							urls = append(urls, cleanedUrl)
						}
					}
				}

				if err := fs.updateHostsFile(urls); err != nil {
					log.Printf("❌ Error updating hosts file: %v", err)
				} else {
					log.Printf("✅ Hosts file updated with %d URLs", len(urls))
				}

				go fs.updatePCStatus()
			} else {
				fs.mutex.Unlock()
				consecutiveNoChanges++

				// Implement exponential backoff when no changes
				if consecutiveNoChanges > 5 && pollInterval < maxInterval {
					pollInterval = time.Duration(float64(pollInterval) * 1.2)
					if pollInterval > maxInterval {
						pollInterval = maxInterval
					}
					ticker.Reset(pollInterval)
					log.Printf("[FIREBASE] Adjusting poll interval to %v (no changes for %d cycles)", pollInterval, consecutiveNoChanges)
				}
			}

		case <-fs.ctx.Done():
			return
		}
	}
}

// fallbackToPolling provides a backup mechanism if real-time listeners fail
func (fs *FirebaseService) fallbackToPolling() {
	ref := fs.client.NewRef(fmt.Sprintf("kidsafe/families/%s/blockedUrls", fs.familyID))
	ticker := time.NewTicker(10 * time.Second) // Less frequent polling as fallback
	defer ticker.Stop()

	log.Printf("📡 Firebase fallback polling started (every 10s)")

	for {
		select {
		case <-ticker.C:
			var urlsData map[string]*BlockedUrl
			if err := ref.Get(fs.ctx, &urlsData); err != nil {
				log.Printf("❌ Error in fallback polling: %v", err)
				continue
			}

			fs.mutex.Lock()
			changed := len(fs.blockedUrls) != len(urlsData)
			if !changed {
				// Check for additions or modifications
				for k, v := range urlsData {
					if existing, ok := fs.blockedUrls[k]; !ok || existing.URL != v.URL || existing.Status != v.Status {
						changed = true
						break
					}
				}

				// Check for deletions (items that exist in fs.blockedUrls but not in urlsData)
				if !changed {
					for k := range fs.blockedUrls {
						if _, exists := urlsData[k]; !exists {
							changed = true
							log.Printf("🗑️ Detected deletion in fallback polling: %s no longer in Firebase data", k)
							break
						}
					}
				}
			}

			if changed {
				fs.blockedUrls = urlsData
				if fs.blockedUrls == nil {
					fs.blockedUrls = make(map[string]*BlockedUrl)
				}
				fs.mutex.Unlock()

				var urls []string
				for _, blockedUrl := range urlsData {
					if blockedUrl != nil && blockedUrl.Status == "active" {
						cleanedUrl := fs.extractDomain(blockedUrl.URL)
						if cleanedUrl != "" {
							urls = append(urls, cleanedUrl)
						}
					}
				}

				if err := fs.updateHostsFile(urls); err != nil {
					log.Printf("❌ Error updating hosts file: %v", err)
				} else {
					log.Printf("📝 Hosts file updated via polling with %d URLs", len(urls))
				}

				go fs.updatePCStatus()
			} else {
				fs.mutex.Unlock()
			}

		case <-fs.ctx.Done():
			return
		}
	}
}

// extractDomain extracts domain from URL for hosts file
func (fs *FirebaseService) extractDomain(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove www prefix
	url = strings.TrimPrefix(url, "www.")

	// Get domain part (remove path)
	parts := strings.Split(url, "/")
	domain := parts[0]

	// Remove port if present
	parts = strings.Split(domain, ":")
	domain = parts[0]

	// Validate domain
	if domain == "" || strings.Contains(domain, " ") {
		return ""
	}

	return domain
}

// updateHostsFile updates the hosts file with new URLs and also updates database
func (fs *FirebaseService) updateHostsFile(urls []string) error {
	// Update database with Firebase synced URLs first
	if fs.database != nil {
		// First, remove all existing firebase-sync rules
		_, err := fs.database.Exec("DELETE FROM block_rules WHERE category = 'firebase-sync'")
		if err != nil {
			log.Printf("Warning: Failed to clear existing firebase-sync rules: %v", err)
		}

		// Add new firebase-sync rules
		for _, url := range urls {
			if url != "" {
				_, err := fs.database.Exec(
					"INSERT INTO block_rules (domain, category, profile_id, reason, is_active) VALUES (?, ?, ?, ?, ?)",
					url, "firebase-sync", 1, "Synced from Android app", true)
				if err != nil {
					log.Printf("Warning: Failed to insert firebase-sync rule for %s: %v", url, err)
				}
			}
		}
		log.Printf("📱 Database updated with %d Firebase synced URLs", len(urls))
	}

	// Now sync ALL rules (Firebase + manual) to hosts file using core service
	if fs.coreService != nil {
		err := fs.coreService.syncRulesToHosts()
		if err != nil {
			return fmt.Errorf("failed to sync all rules to hosts file: %v", err)
		}
		log.Printf("✅ All rules (Firebase + manual) synced to hosts file")

		// Broadcast real-time update to web UI clients
		go fs.coreService.broadcastRulesUpdate()
		log.Printf("📡 Broadcasting Firebase rules update to web UI clients")
	} else {
		log.Printf("⚠️ Core service not available, falling back to Firebase-only sync")
		// Fallback to old method if core service not available
		if fs.hostsManager == nil {
			return fmt.Errorf("hosts manager not available")
		}
		err := fs.hostsManager.UpdateBlockedDomains(urls)
		if err != nil {
			return fmt.Errorf("failed to update hosts file: %v", err)
		}
	}

	return nil
}

// updatePCStatus updates the PC status in Firebase
func (fs *FirebaseService) updatePCStatus() {
	ref := fs.client.NewRef(fmt.Sprintf("kidsafe/families/%s/pcStatus", fs.familyID))

	// Get current blocked count
	fs.mutex.Lock()
	blockedCount := len(fs.blockedUrls)
	fs.mutex.Unlock()

	// Determine host file status
	hostFileStatus := "active"
	if blockedCount == 0 {
		hostFileStatus = "inactive"
	}

	status := &PCStatus{
		LastSeen:       time.Now().UnixMilli(),
		Status:         "connected",
		Version:        "1.0.0",
		HostFileStatus: hostFileStatus,
		BlockedCount:   blockedCount,
	}

	err := ref.Set(fs.ctx, status)
	if err != nil {
		log.Printf("Error updating PC status: %v", err)
	} else {
		log.Printf("[FIREBASE] PC status updated: %d blocked URLs", blockedCount)
	}
}

// updatePCStatusPeriodically updates PC status every 30 seconds
func (fs *FirebaseService) updatePCStatusPeriodically() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fs.updatePCStatus()
		case <-fs.ctx.Done():
			return
		}
	}
}

// GetBlockedUrls returns the current list of blocked URLs
func (fs *FirebaseService) GetBlockedUrls() map[string]*BlockedUrl {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// Create a copy to avoid race conditions
	result := make(map[string]*BlockedUrl)
	for k, v := range fs.blockedUrls {
		if v != nil {
			urlCopy := *v
			result[k] = &urlCopy
		}
	}

	return result
}

// TestConnection tests the Firebase connection
func (fs *FirebaseService) TestConnection() error {
	ref := fs.client.NewRef(fmt.Sprintf("kidsafe/families/%s/connectionTest", fs.familyID))

	testData := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
		"message":   "PC connection test",
	}

	err := ref.Set(fs.ctx, testData)
	if err != nil {
		return fmt.Errorf("Firebase connection test failed: %v", err)
	}

	log.Println("✅ Firebase connection test successful")
	return nil
}

// GetStats returns Firebase sync statistics
func (fs *FirebaseService) GetStats() map[string]interface{} {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// Count active time rules
	activeTimeRules := 0
	for _, rule := range fs.timeRules {
		if rule != nil && rule.Active {
			activeTimeRules++
		}
	}

	return map[string]interface{}{
		"family_id":         fs.familyID,
		"user_email":        fs.userEmail,
		"is_listening":      fs.isListening,
		"blocked_count":     len(fs.blockedUrls),
		"time_rules_count":  len(fs.timeRules),
		"active_time_rules": activeTimeRules,
		"last_updated":      time.Now().UnixMilli(),
		"status":            "connected",
	}
}

// ForceSync manually triggers a Firebase sync check
func (fs *FirebaseService) ForceSync() error {
	log.Println("🔄 Manual Firebase sync triggered...")

	// Calculate paths to check
	possiblePaths := []string{
		fmt.Sprintf("kidsafe/families/%s/blockedUrls", fs.familyID),
		"kidsafe/blockedUrls",
		fmt.Sprintf("kidsafe/blockedUrls_%s", fs.familyID),
		fmt.Sprintf("families/%s/blockedUrls", fs.familyID),
		fmt.Sprintf("users/%s/blockedUrls", fs.familyID),
		"blockedUrls",
	}

	if fs.userEmail != "" {
		localAuthUID := generateLocalAuthUID(fs.userEmail)
		if localAuthUID != fs.familyID {
			possiblePaths = append(possiblePaths, fmt.Sprintf("kidsafe/families/%s/blockedUrls", localAuthUID))
			possiblePaths = append(possiblePaths, fmt.Sprintf("families/%s/blockedUrls", localAuthUID))
		}
	}

	log.Printf("🔍 Checking %d paths for Firebase data...", len(possiblePaths))

	for i, path := range possiblePaths {
		log.Printf("   Checking path %d: %s", i+1, path)
		ref := fs.client.NewRef(path)

		var urlsData map[string]*BlockedUrl
		if err := ref.Get(fs.ctx, &urlsData); err != nil {
			log.Printf("     ❌ Error: %v", err)
			continue
		}

		if len(urlsData) > 0 {
			log.Printf("     ✅ Found %d URLs at this path!", len(urlsData))

			// Process the data
			var urls []string
			for _, blockedUrl := range urlsData {
				if blockedUrl != nil && blockedUrl.Status == "active" {
					cleanedUrl := fs.extractDomain(blockedUrl.URL)
					if cleanedUrl != "" {
						urls = append(urls, cleanedUrl)
					}
				}
			}

			if len(urls) > 0 {
				if err := fs.updateHostsFile(urls); err != nil {
					return fmt.Errorf("failed to update hosts file: %v", err)
				}
				log.Printf("✅ Manual sync successful: %d URLs updated", len(urls))
				return nil
			}
		} else {
			log.Printf("     ⚪ No data found")
		}
	}

	return fmt.Errorf("no Firebase data found in any of the %d paths checked", len(possiblePaths))
}

// Helper function to setup Firebase service
func SetupFirebaseService(userUID string, hostsManager *HostsManager, database *sql.DB, coreService *CoreService) (*FirebaseService, error) {
	return SetupFirebaseServiceWithEmail(userUID, "", hostsManager, database, coreService)
}

// SetupFirebaseServiceWithEmail creates Firebase service with email for LocalAuth UID calculation
func SetupFirebaseServiceWithEmail(userUID, userEmail string, hostsManager *HostsManager, database *sql.DB, coreService *CoreService) (*FirebaseService, error) {
	// Look for Firebase credentials in common locations
	credentialsPaths := []string{
		"firebase-credentials.json",
		"kidsafe-firebase-credentials.json",
		filepath.Join("data", "firebase-credentials.json"),
		filepath.Join("..", "firebase-credentials.json"),
	}

	var credentialsPath string
	for _, path := range credentialsPaths {
		if fileExists(path) {
			credentialsPath = path
			break
		}
	}

	if credentialsPath == "" {
		return nil, fmt.Errorf("Firebase credentials file not found. Please place it as 'firebase-credentials.json'")
	}

	log.Printf("Using Firebase credentials: %s", credentialsPath)

	firebaseService, err := NewFirebaseService(credentialsPath, userUID, hostsManager, database, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase service: %v", err)
	}

	// Set the email if provided
	if userEmail != "" {
		firebaseService.userEmail = userEmail
		log.Printf("📧 Firebase service configured with email: %s", userEmail)
	}

	// Set the core service reference if provided
	if coreService != nil {
		firebaseService.SetCoreService(coreService)
		log.Printf("🔗 Firebase service linked to core service")
	}

	// Test connection
	err = firebaseService.TestConnection()
	if err != nil {
		return nil, fmt.Errorf("Firebase connection test failed: %v", err)
	}

	return firebaseService, nil
}

// SetupFirebaseServiceAnonymous creates Firebase service for public read access
func SetupFirebaseServiceAnonymous(userUID string, hostsManager *HostsManager, database *sql.DB) (*FirebaseService, error) {
	// Use built-in config for anonymous access
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize Firebase app with public database URL only
	firebaseConfig := &firebase.Config{
		DatabaseURL: "https://kidsafe-control-default-rtdb.asia-southeast1.firebasedatabase.app/",
		ProjectID:   "kidsafe-control",
	}

	app, err := firebase.NewApp(ctx, firebaseConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error initializing Firebase app: %v", err)
	}

	// Get database client
	client, err := app.Database(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error getting Database client: %v", err)
	}

	fs := &FirebaseService{
		app:          app,
		client:       client,
		familyID:     userUID,
		hostsManager: hostsManager,
		database:     database,
		ctx:          ctx,
		cancel:       cancel,
		isListening:  false,
		blockedUrls:  make(map[string]*BlockedUrl),
	}

	log.Printf("🔥 Anonymous Firebase service initialized for user: %s", userUID)
	log.Printf("⚠️ Note: This mode has read-only access to public data")
	return fs, nil
}

// Helper function to check if file exists
func fileExists(filename string) bool {
	_, err := filepath.Abs(filename)
	return err == nil
}

// syncToLocalDatabase syncs Firebase URLs to local SQLite database
func (fs *FirebaseService) syncToLocalDatabase(firebaseUrls map[string]*BlockedUrl) error {
	if fs.database == nil {
		return fmt.Errorf("database not available")
	}

	log.Printf("🔄 Starting sync of %d Firebase URLs to local database", len(firebaseUrls))

	// First, get all Firebase domains currently active
	activeFirebaseDomains := make(map[string]bool)

	// Process each Firebase URL
	for _, blockedUrl := range firebaseUrls {
		if blockedUrl == nil || blockedUrl.Status != "active" {
			continue
		}

		domain := fs.extractDomain(blockedUrl.URL)
		if domain == "" {
			continue
		}

		activeFirebaseDomains[domain] = true

		// Check if domain already exists in local database
		var existingID int
		err := fs.database.QueryRow("SELECT id FROM block_rules WHERE domain = ? AND category = 'firebase-sync'", domain).Scan(&existingID)

		if err == sql.ErrNoRows {
			// Domain doesn't exist, insert it
			_, insertErr := fs.database.Exec(
				"INSERT INTO block_rules (domain, category, profile_id, reason, created_at, is_active) VALUES (?, ?, ?, ?, datetime('now'), ?)",
				domain,
				"firebase-sync",           // Category for Firebase synced rules
				1,                         // Default profile ID
				"Synced from Android app", // Reason
				true,                      // Active
			)
			if insertErr != nil {
				log.Printf("❌ Failed to insert Firebase domain %s: %v", domain, insertErr)
			} else {
				log.Printf("✅ Added Firebase domain to local DB: %s", domain)
			}
		} else if err != nil {
			log.Printf("❌ Error checking existing domain %s: %v", domain, err)
		} else {
			// Domain exists, ensure it's active
			fs.database.Exec("UPDATE block_rules SET is_active = 1 WHERE id = ?", existingID)
		}
	}

	// Remove Firebase domains that are no longer active (deleted from Android)
	rows, err := fs.database.Query("SELECT id, domain FROM block_rules WHERE category = 'firebase-sync' AND is_active = 1")
	if err != nil {
		log.Printf("❌ Error querying Firebase domains: %v", err)
		return err
	}
	defer rows.Close()

	var domainsToRemove []struct {
		ID     int
		Domain string
	}

	for rows.Next() {
		var id int
		var domain string
		if err := rows.Scan(&id, &domain); err != nil {
			continue
		}

		// If this domain is not in the active Firebase list, mark for removal
		if !activeFirebaseDomains[domain] {
			domainsToRemove = append(domainsToRemove, struct {
				ID     int
				Domain string
			}{id, domain})
		}
	}

	// Remove domains that were deleted from Android
	for _, item := range domainsToRemove {
		_, err := fs.database.Exec("DELETE FROM block_rules WHERE id = ?", item.ID)
		if err != nil {
			log.Printf("❌ Failed to remove deleted Firebase domain %s: %v", item.Domain, err)
		} else {
			log.Printf("🗑️ Removed deleted Firebase domain from local DB: %s", item.Domain)
		}
	}

	log.Printf("✅ Firebase sync to local database completed: %d active domains, %d removed", len(activeFirebaseDomains), len(domainsToRemove))
	return nil
}

// listenForTimeRules listens for time rules changes from Android app
func (fs *FirebaseService) listenForTimeRules() {
	// Build possible paths for time rules
	possiblePaths := []string{
		fmt.Sprintf("%s/syncStatus/timeRules", fs.familyID),       // Real Android path structure
		fmt.Sprintf("kidsafe/families/%s/timeRules", fs.familyID), // Main Firebase Auth UID path
		fmt.Sprintf("families/%s/timeRules", fs.familyID),         // Alternative structure
		fmt.Sprintf("users/%s/timeRules", fs.familyID),            // Alternative structure
	}

	// Add LocalAuth UID path if we have an email
	if fs.userEmail != "" {
		localAuthUID := generateLocalAuthUID(fs.userEmail)
		if localAuthUID != fs.familyID {
			possiblePaths = append(possiblePaths, fmt.Sprintf("kidsafe/families/%s/timeRules", localAuthUID))
			log.Printf("🕐 Also checking LocalAuth UID path for time rules: %s", localAuthUID)
		}
	}

	log.Printf("🕐 Starting time rules listener for family: %s", fs.familyID)
	log.Printf("📡 Will check %d paths for time rules...", len(possiblePaths))

	// Debug: Print all paths being checked
	for i, path := range possiblePaths {
		log.Printf("   Time Rules Path %d: %s", i+1, path)
	}

	// Start polling for time rules
	fs.pollTimeRules(possiblePaths)
}

// pollTimeRules polls Firebase for time rules changes
func (fs *FirebaseService) pollTimeRules(paths []string) {
	pollInterval := 3 * time.Second
	maxInterval := 30 * time.Second

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	consecutiveNoChanges := 0
	var activePath string
	lastRulesHash := ""

	for {
		select {
		case <-ticker.C:
			var foundRules map[string]*AndroidTimeRule
			var workingPath string

			// Try each path until we find data
			for _, path := range paths {
				ref := fs.client.NewRef(path)
				var rulesData map[string]*AndroidTimeRule

				if err := ref.Get(fs.ctx, &rulesData); err != nil {
					continue // Try next path
				}

				if len(rulesData) > 0 {
					foundRules = rulesData
					workingPath = path

					// If this is a new working path, log it
					if activePath != workingPath {
						log.Printf("✅ Found time rules at path: %s (%d rules)", workingPath, len(rulesData))
						activePath = workingPath

						// Debug: Print rules found
						for key, rule := range rulesData {
							if rule != nil && rule.Active {
								log.Printf("   Rule %s: %s - daily limit: %d min", key, rule.Name, rule.DailyLimitMinutes)
							}
						}
					}
					break
				}
			}

			if foundRules == nil {
				consecutiveNoChanges++
				if consecutiveNoChanges > 10 && pollInterval < maxInterval {
					pollInterval = time.Duration(float64(pollInterval) * 1.5)
					if pollInterval > maxInterval {
						pollInterval = maxInterval
					}
					ticker.Reset(pollInterval)
				}
				continue
			}

			// Check if rules have changed (simple hash comparison)
			currentHash := fs.calculateTimeRulesHash(foundRules)
			if currentHash != lastRulesHash {
				log.Printf("🕐 Time rules changed, applying updates...")
				fs.processTimeRulesUpdate(foundRules)
				lastRulesHash = currentHash
				consecutiveNoChanges = 0

				// Reset to fast polling after changes
				if pollInterval > 3*time.Second {
					pollInterval = 3 * time.Second
					ticker.Reset(pollInterval)
				}
			} else {
				consecutiveNoChanges++
			}

		case <-fs.ctx.Done():
			log.Println("🕐 Time rules listener stopped")
			return
		}
	}
}

// calculateTimeRulesHash creates a simple hash of time rules for change detection
func (fs *FirebaseService) calculateTimeRulesHash(rules map[string]*AndroidTimeRule) string {
	var hash strings.Builder
	for key, rule := range rules {
		if rule != nil {
			hash.WriteString(fmt.Sprintf("%s:%v:%d:%d:%d:%d",
				key, rule.Active, rule.DailyLimitMinutes, rule.BreakIntervalMinutes,
				rule.BreakDurationMinutes, rule.UpdatedAt))
		}
	}
	return hash.String()
}

// processTimeRulesUpdate processes time rules from Android and applies to TimeManager
func (fs *FirebaseService) processTimeRulesUpdate(androidRules map[string]*AndroidTimeRule) {
	fs.mutex.Lock()
	fs.timeRules = androidRules
	fs.mutex.Unlock()

	// Convert Android rules to PC format
	pcRules := fs.convertAndroidRulesToPCFormat(androidRules)

	// Apply to TimeManager if available
	if fs.coreService != nil && fs.coreService.timeManager != nil {
		log.Printf("🕐 Applying %d time rules to TimeManager", len(androidRules))
		fs.coreService.timeManager.UpdateRules(*pcRules)
	} else {
		log.Printf("⚠️ TimeManager not available, time rules stored but not applied")
	}
}

// convertAndroidRulesToPCFormat converts Android time rules to PC TimeRules format
func (fs *FirebaseService) convertAndroidRulesToPCFormat(androidRules map[string]*AndroidTimeRule) *TimeRules {
	// Initialize default rules
	pcRules := &TimeRules{
		Weekdays: DayRule{
			Enabled:              false,
			DailyLimitMinutes:    0,
			BreakIntervalMinutes: 0,
			BreakDurationMinutes: 0,
			AllowedSlots:         []TimeSlot{},
		},
		Weekends: DayRule{
			Enabled:              false,
			DailyLimitMinutes:    0,
			BreakIntervalMinutes: 0,
			BreakDurationMinutes: 0,
			AllowedSlots:         []TimeSlot{},
		},
	}

	// Process Android rules
	var hasActiveRules bool
	var maxDailyLimit int
	var maxBreakInterval int
	var maxBreakDuration int

	for _, rule := range androidRules {
		if rule == nil || !rule.Active {
			continue
		}

		hasActiveRules = true

		// Find the most restrictive (highest) values
		if rule.DailyLimitMinutes > maxDailyLimit {
			maxDailyLimit = rule.DailyLimitMinutes
		}
		if rule.BreakIntervalMinutes > maxBreakInterval {
			maxBreakInterval = rule.BreakIntervalMinutes
		}
		if rule.BreakDurationMinutes > maxBreakDuration {
			maxBreakDuration = rule.BreakDurationMinutes
		}

		log.Printf("🕐 Processing rule: %s (type: %s, daily limit: %d min)",
			rule.Name, rule.RuleType, rule.DailyLimitMinutes)
	}

	if hasActiveRules {
		// Apply same rules to both weekdays and weekends for now
		// TODO: In future, Android could send separate weekend/weekday rules
		dayRule := DayRule{
			Enabled:              true,
			DailyLimitMinutes:    maxDailyLimit,
			BreakIntervalMinutes: maxBreakInterval,
			BreakDurationMinutes: maxBreakDuration,
			AllowedSlots:         []TimeSlot{}, // Default: no time restrictions, only daily limit
		}

		// If no daily limit is set, allow all day but with breaks
		if maxDailyLimit == 0 {
			dayRule.AllowedSlots = []TimeSlot{
				{StartTime: "00:00", EndTime: "23:59"}, // Allow all day
			}
		}

		pcRules.Weekdays = dayRule
		pcRules.Weekends = dayRule

		log.Printf("🕐 Converted rules: daily limit=%d min, break interval=%d min, break duration=%d min",
			maxDailyLimit, maxBreakInterval, maxBreakDuration)
	} else {
		log.Printf("🕐 No active time rules found")
	}

	return pcRules
}

// GetTimeRules returns current time rules from Android
func (fs *FirebaseService) GetTimeRules() map[string]*AndroidTimeRule {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// Return a copy to avoid race conditions
	result := make(map[string]*AndroidTimeRule)
	for k, v := range fs.timeRules {
		result[k] = v
	}
	return result
}

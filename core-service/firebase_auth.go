package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// RealFirebaseAuth handles actual Firebase Authentication
type RealFirebaseAuth struct {
	apiKey       string
	idToken      string
	refreshToken string
	uid          string
	email        string
	expiresAt    time.Time
}

// FirebaseAuthResponse from Firebase Auth API
type FirebaseAuthResponse struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	LocalID      string `json:"localId"`
	Email        string `json:"email"`
	ExpiresIn    string `json:"expiresIn"`
}

// NewRealFirebaseAuth creates a new Firebase Auth instance
func NewRealFirebaseAuth() (*RealFirebaseAuth, error) {
	// Load API key from config
	apiKey, err := loadFirebaseWebAPIKey()
	if err != nil {
		return nil, err
	}

	auth := &RealFirebaseAuth{
		apiKey: apiKey,
	}

	// Try to load saved token
	if err := auth.loadSavedToken(); err == nil && auth.isTokenValid() {
		log.Printf("✅ Loaded saved Firebase token for user: %s", auth.email)
		return auth, nil
	}

	return auth, nil
}

// Login authenticates with Firebase using email/password
func (auth *RealFirebaseAuth) Login(email, password string) error {
	log.Printf("[FIREBASE-AUTH] Starting login for email: %s", email)
	url := fmt.Sprintf("https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s", auth.apiKey)

	payload := map[string]string{
		"email":             email,
		"password":          password,
		"returnSecureToken": "true",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		log.Printf("[FIREBASE-AUTH] HTTP error response: %d", resp.StatusCode)
		log.Printf("[FIREBASE-AUTH] Response body: %s", string(body))
		var errorResp map[string]interface{}
		json.Unmarshal(body, &errorResp)
		if errData, ok := errorResp["error"].(map[string]interface{}); ok {
			if msg, ok := errData["message"].(string); ok {
				return fmt.Errorf("Firebase Auth error: %s", msg)
			}
		}
		return fmt.Errorf("authentication failed with status %d", resp.StatusCode)
	}

	var authResp FirebaseAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		log.Printf("[FIREBASE-AUTH] JSON unmarshal error: %v", err)
		log.Printf("[FIREBASE-AUTH] Response body: %s", string(body))
		return err
	}

	log.Printf("[FIREBASE-AUTH] Login successful for: %s", email)

	// Save auth data
	auth.idToken = authResp.IDToken
	auth.refreshToken = authResp.RefreshToken
	auth.uid = authResp.LocalID
	auth.email = authResp.Email

	// Parse expiry (Firebase returns seconds as string)
	expiresIn := 3600 // default 1 hour
	fmt.Sscanf(authResp.ExpiresIn, "%d", &expiresIn)
	auth.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)

	// Save to file for persistence
	auth.saveToken()

	log.Printf("✅ Firebase Auth successful - UID: %s, Email: %s", auth.uid, auth.email)
	return nil
}

// RefreshAuthToken refreshes the Firebase ID token
func (auth *RealFirebaseAuth) RefreshAuthToken() error {
	if auth.refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	url := fmt.Sprintf("https://securetoken.googleapis.com/v1/token?key=%s", auth.apiKey)

	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": auth.refreshToken,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if newToken, ok := result["id_token"].(string); ok {
		auth.idToken = newToken

		// Update expiry
		if expiresIn, ok := result["expires_in"].(string); ok {
			seconds := 3600
			fmt.Sscanf(expiresIn, "%d", &seconds)
			auth.expiresAt = time.Now().Add(time.Duration(seconds) * time.Second)
		}

		auth.saveToken()
		log.Println("✅ Firebase token refreshed successfully")
	}

	return nil
}

// GetUID returns the Firebase user UID
func (auth *RealFirebaseAuth) GetUID() string {
	return auth.uid
}

// GetEmail returns the user email
func (auth *RealFirebaseAuth) GetEmail() string {
	return auth.email
}

// IsAuthenticated checks if user is authenticated
func (auth *RealFirebaseAuth) IsAuthenticated() bool {
	return auth.uid != "" && auth.isTokenValid()
}

// isTokenValid checks if the current token is still valid
func (auth *RealFirebaseAuth) isTokenValid() bool {
	if auth.idToken == "" {
		return false
	}
	// Check if token expired (with 5 minute buffer)
	return time.Now().Before(auth.expiresAt.Add(-5 * time.Minute))
}

// EnsureValidToken ensures we have a valid token, refreshing if needed
func (auth *RealFirebaseAuth) EnsureValidToken() error {
	if !auth.isTokenValid() && auth.refreshToken != "" {
		return auth.RefreshAuthToken()
	}
	if auth.idToken == "" {
		return fmt.Errorf("not authenticated")
	}
	return nil
}

// saveToken saves auth data to file
func (auth *RealFirebaseAuth) saveToken() error {
	data := map[string]interface{}{
		"uid":          auth.uid,
		"email":        auth.email,
		"idToken":      auth.idToken,
		"refreshToken": auth.refreshToken,
		"expiresAt":    auth.expiresAt.Unix(),
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("firebase_auth_token.json", jsonData, 0600)
}

// loadSavedToken loads previously saved auth data
func (auth *RealFirebaseAuth) loadSavedToken() error {
	data, err := os.ReadFile("firebase_auth_token.json")
	if err != nil {
		return err
	}

	var saved map[string]interface{}
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}

	// Load fields
	if uid, ok := saved["uid"].(string); ok {
		auth.uid = uid
	}
	if email, ok := saved["email"].(string); ok {
		auth.email = email
	}
	if token, ok := saved["idToken"].(string); ok {
		auth.idToken = token
	}
	if refresh, ok := saved["refreshToken"].(string); ok {
		auth.refreshToken = refresh
	}
	if expires, ok := saved["expiresAt"].(float64); ok {
		auth.expiresAt = time.Unix(int64(expires), 0)
	}

	return nil
}

// loadFirebaseWebAPIKey loads the Web API key from config
func loadFirebaseWebAPIKey() (string, error) {
	// Get current working directory for debugging
	cwd, _ := os.Getwd()
	log.Printf("🔍 Loading Firebase config from working directory: %s", cwd)

	// Get executable directory for more reliable path resolution
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)

	// Try multiple locations
	configPaths := []string{
		"firebase-config.json",                         // current working directory
		filepath.Join(execDir, "firebase-config.json"), // same dir as executable
		"../ui-admin/firebase-config.json",
		"firebase-credentials.json",                                      // sometimes stored here
		"core-service/firebase-config.json",                              // from project root
		"./core-service/firebase-config.json",                            // from project root
		filepath.Join(execDir, "..", "ui-admin", "firebase-config.json"), // relative to exe
		filepath.Join(execDir, "..", "firebase-config.json"),             // parent of exe dir
	}

	for _, path := range configPaths {
		log.Printf("🔍 Trying config path: %s", path)
		if data, err := os.ReadFile(path); err == nil {
			log.Printf("✅ Found config file at: %s", path)
			var config map[string]interface{}
			if err := json.Unmarshal(data, &config); err == nil {
				if apiKey, ok := config["apiKey"].(string); ok && apiKey != "" {
					log.Printf("✅ Successfully loaded Firebase API key from: %s", path)
					return apiKey, nil
				} else {
					log.Printf("⚠️ Config file found but no apiKey field: %s", path)
				}
			} else {
				log.Printf("⚠️ Config file found but JSON parse failed: %s - %v", path, err)
			}
		} else {
			log.Printf("❌ Config file not found: %s - %v", path, err)
		}
	}

	// Try from environment variable
	if apiKey := os.Getenv("FIREBASE_API_KEY"); apiKey != "" {
		log.Printf("✅ Using Firebase API key from environment variable")
		return apiKey, nil
	}

	log.Printf("❌ Firebase Web API key not found in any location")
	return "", fmt.Errorf("Firebase Web API key not found. Please add it to firebase-config.json")
}

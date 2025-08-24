// Database Utilities for KidSafe PC
// This file provides utility functions for database operations
// Use these functions from main.go or create separate utility commands

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

// DatabaseUtils provides utility functions for database operations
type DatabaseUtils struct {
	dbPath string
}

// NewDatabaseUtils creates a new DatabaseUtils instance
func NewDatabaseUtils(dbPath string) *DatabaseUtils {
	return &DatabaseUtils{dbPath: dbPath}
}

// CheckDatabase displays all rules in the database
func (du *DatabaseUtils) CheckDatabase() error {
	db, err := sql.Open("sqlite3", du.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Println("🔍 Checking database contents...")
	fmt.Println("================================")

	rows, err := db.Query("SELECT id, domain, category, reason, is_active FROM block_rules ORDER BY category, id")
	if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	fmt.Println("📋 All Block Rules:")
	count := 0
	firebaseCount := 0
	for rows.Next() {
		var id int
		var domain, category, reason string
		var isActive bool

		err := rows.Scan(&id, &domain, &category, &reason, &isActive)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		status := "❌"
		if isActive {
			status = "✅"
		}

		emoji := "🌐"
		if category == "firebase-sync" {
			emoji = "📱"
			firebaseCount++
		}

		fmt.Printf("  %s %s [%d] %s (%s) - %s\n", status, emoji, id, domain, category, reason)
		count++
	}

	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("  Total rules: %d\n", count)
	fmt.Printf("  Firebase rules: %d\n", firebaseCount)
	fmt.Printf("  Other rules: %d\n", count-firebaseCount)

	return nil
}

// SyncFirebaseUrls manually adds Firebase URLs to database
func (du *DatabaseUtils) SyncFirebaseUrls(urls []string) error {
	db, err := sql.Open("sqlite3", du.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Println("🔄 Manual Firebase Sync Tool")
	fmt.Println("============================")

	// Clear existing firebase-sync rules
	fmt.Println("🗑️ Clearing existing Firebase sync rules...")
	_, err = db.Exec("DELETE FROM block_rules WHERE category = 'firebase-sync'")
	if err != nil {
		log.Printf("Warning: Failed to clear existing firebase-sync rules: %v", err)
	}

	// Add new firebase-sync rules
	fmt.Printf("📱 Adding %d URLs from Firebase...\n", len(urls))
	for _, url := range urls {
		_, err := db.Exec(
			"INSERT INTO block_rules (domain, category, profile_id, reason, is_active) VALUES (?, ?, ?, ?, ?)",
			url, "firebase-sync", 1, "Synced from Android app", true)
		if err != nil {
			log.Printf("❌ Failed to insert firebase-sync rule for %s: %v", url, err)
		} else {
			fmt.Printf("✅ Added: %s\n", url)
		}
	}

	fmt.Printf("\n🎉 Manual sync completed! Added %d URLs to database.\n", len(urls))
	fmt.Println("💡 These URLs will now appear in the PC UI with 'firebase-sync' category.")
	fmt.Println("🔄 Refresh the UI to see the changes.")

	return nil
}

// TestAPILogic simulates the API logic to test database queries
func (du *DatabaseUtils) TestAPILogic() error {
	db, err := sql.Open("sqlite3", du.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Println("🧪 Testing API Logic...")
	fmt.Println("=======================")

	// Simulate the exact query from handleGetRules
	rows, err := db.Query("SELECT id, domain, category, profile_id, reason, created_at, is_active FROM block_rules ORDER BY created_at DESC")
	if err != nil {
		return fmt.Errorf("failed to query database: %v", err)
	}
	defer rows.Close()

	// Use anonymous struct to avoid conflicts
	var rules []struct {
		ID        int    `json:"id"`
		Domain    string `json:"domain"`
		Category  string `json:"category"`
		ProfileID int    `json:"profile_id"`
		Reason    string `json:"reason"`
		CreatedAt string `json:"created_at"`
		IsActive  bool   `json:"is_active"`
	}

	for rows.Next() {
		var rule struct {
			ID        int    `json:"id"`
			Domain    string `json:"domain"`
			Category  string `json:"category"`
			ProfileID int    `json:"profile_id"`
			Reason    string `json:"reason"`
			CreatedAt string `json:"created_at"`
			IsActive  bool   `json:"is_active"`
		}
		err := rows.Scan(&rule.ID, &rule.Domain, &rule.Category, &rule.ProfileID, &rule.Reason, &rule.CreatedAt, &rule.IsActive)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		rules = append(rules, rule)
	}

	fmt.Printf("📋 Found %d rules\n", len(rules))

	// Convert to JSON like the API does
	jsonData, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	fmt.Println("\n📄 JSON Output:")
	fmt.Println(string(jsonData))

	if len(rules) == 0 {
		fmt.Println("\n⚠️ No rules found - this explains why API returns null!")
	} else {
		fmt.Printf("\n✅ API should return %d rules\n", len(rules))
	}

	return nil
}

// Helper functions to use these utilities from main.go

// CheckDatabaseContents is a helper to check database contents
func CheckDatabaseContents() {
	dbUtils := NewDatabaseUtils("./data/parental_control.db")
	if err := dbUtils.CheckDatabase(); err != nil {
		log.Printf("Error checking database: %v", err)
	}
}

// SyncSampleFirebaseUrls is a helper to sync sample Firebase URLs
func SyncSampleFirebaseUrls() {
	sampleUrls := []string{
		"facebook.com",
		"instagram.com",
		"tiktok.com",
		"youtube.com",
	}
	dbUtils := NewDatabaseUtils("./data/parental_control.db")
	if err := dbUtils.SyncFirebaseUrls(sampleUrls); err != nil {
		log.Printf("Error syncing Firebase URLs: %v", err)
	}
}

// TestDatabaseAPI is a helper to test API logic
func TestDatabaseAPI() {
	dbUtils := NewDatabaseUtils("./data/parental_control.db")
	if err := dbUtils.TestAPILogic(); err != nil {
		log.Printf("Error testing API logic: %v", err)
	}
}

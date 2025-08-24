package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	WindowsHostsPath = `C:\Windows\System32\drivers\etc\hosts`
	BackupSuffix     = ".kidSafe_backup"
	BlockedIP        = "127.0.0.1" // Redirect blocked domains to localhost
)

type HostsManager struct {
	mutex          sync.RWMutex
	originalHosts  string
	blockedDomains map[string]bool
	backupPath     string
}

func NewHostsManager() *HostsManager {
	return &HostsManager{
		blockedDomains: make(map[string]bool),
		backupPath:     WindowsHostsPath + BackupSuffix,
	}
}

// Initialize creates backup and prepares hosts manager
func (hm *HostsManager) Initialize() error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	log.Println("Initializing Hosts Manager...")

	// Read original hosts file
	content, err := hm.readHostsFile()
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %v", err)
	}
	hm.originalHosts = content

	// Create backup
	if err := hm.createBackup(); err != nil {
		log.Printf("Warning: Failed to create backup: %v", err)
	}

	log.Println("Hosts Manager initialized successfully")
	return nil
}

// AddBlockedDomain adds a domain to the blocked list and updates hosts file
func (hm *HostsManager) AddBlockedDomain(domain string) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// Add to internal map
	hm.blockedDomains[domain] = true

	// Also block www variant
	if !strings.HasPrefix(domain, "www.") {
		hm.blockedDomains["www."+domain] = true
	}

	log.Printf("Added blocked domain: %s", domain)
	return hm.updateHostsFile()
}

// RemoveBlockedDomain removes a domain from blocked list and updates hosts file
func (hm *HostsManager) RemoveBlockedDomain(domain string) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	domain = strings.ToLower(strings.TrimSpace(domain))

	// Remove from internal map
	delete(hm.blockedDomains, domain)
	delete(hm.blockedDomains, "www."+domain)

	log.Printf("Removed blocked domain: %s", domain)
	return hm.updateHostsFile()
}

// UpdateBlockedDomains replaces all blocked domains with new list
func (hm *HostsManager) UpdateBlockedDomains(domains []string) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	// Clear current blocked domains
	hm.blockedDomains = make(map[string]bool)

	// Add new domains
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" {
			hm.blockedDomains[domain] = true
			// Also block www variant
			if !strings.HasPrefix(domain, "www.") {
				hm.blockedDomains["www."+domain] = true
			}
		}
	}

	log.Printf("Updated blocked domains list: %d domains", len(domains))
	return hm.updateHostsFile()
}

// GetBlockedDomains returns list of currently blocked domains
func (hm *HostsManager) GetBlockedDomains() []string {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	var domains []string
	for domain := range hm.blockedDomains {
		// Skip www variants in the list to avoid duplicates
		if !strings.HasPrefix(domain, "www.") {
			domains = append(domains, domain)
		}
	}
	return domains
}

// IsBlocked checks if a domain is currently blocked
func (hm *HostsManager) IsBlocked(domain string) bool {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	return hm.blockedDomains[domain]
}

// RestoreOriginal restores the original hosts file from backup
func (hm *HostsManager) RestoreOriginal() error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	log.Println("Restoring original hosts file...")

	// If we have backup, restore from it
	if _, err := os.Stat(hm.backupPath); err == nil {
		if err := hm.copyFile(hm.backupPath, WindowsHostsPath); err != nil {
			return fmt.Errorf("failed to restore from backup: %v", err)
		}
		// Remove backup file
		os.Remove(hm.backupPath)
	} else {
		// Otherwise restore from memory
		if err := hm.writeHostsFile(hm.originalHosts); err != nil {
			return fmt.Errorf("failed to restore original content: %v", err)
		}
	}

	// Clear blocked domains
	hm.blockedDomains = make(map[string]bool)

	log.Println("Original hosts file restored successfully")
	return nil
}

// Private methods

func (hm *HostsManager) readHostsFile() (string, error) {
	content, err := os.ReadFile(WindowsHostsPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (hm *HostsManager) writeHostsFile(content string) error {
	log.Printf("📝 Attempting to write hosts file (%d bytes)", len(content))

	// Strategy 1: Try direct write first (works if already elevated)
	if err := os.WriteFile(WindowsHostsPath, []byte(content), 0644); err == nil {
		log.Println("✅ Direct hosts file write successful")
		go hm.flushDNSCache()
		return nil
	}

	log.Println("⚠️ Direct write failed, trying elevated methods...")

	// Strategy 2: Try temp file approach
	tempPath := WindowsHostsPath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content), 0644); err == nil {
		if err := os.Rename(tempPath, WindowsHostsPath); err == nil {
			log.Println("✅ Temp file approach successful")
			go hm.flushDNSCache()
			return nil
		}
		os.Remove(tempPath) // Clean up
	}

	log.Println("⚠️ Temp file approach failed, trying PowerShell...")

	// Strategy 3: Use PowerShell with elevation
	if err := hm.writeHostsWithPowerShell(content); err == nil {
		log.Println("✅ PowerShell elevated write successful")
		return nil
	}

	log.Println("⚠️ PowerShell approach failed, trying robocopy...")

	// Strategy 4: Use robocopy as last resort
	if err := hm.writeHostsWithRobocopy(content); err == nil {
		log.Println("✅ Robocopy approach successful")
		return nil
	}

	// All strategies failed
	return fmt.Errorf("all hosts file write strategies failed - check administrator permissions and antivirus settings")
}

// writeHostsWithPowerShell uses PowerShell with elevated permissions
func (hm *HostsManager) writeHostsWithPowerShell(content string) error {
	// Escape content for PowerShell
	escapedContent := strings.ReplaceAll(content, "'", "''")
	escapedContent = strings.ReplaceAll(escapedContent, "`", "``")

	cmd := fmt.Sprintf(`$content = @'
%s
'@; $content | Out-File -FilePath '%s' -Encoding UTF8 -Force`, escapedContent, WindowsHostsPath)

	// Try with PowerShell
	if _, err := hm.runCommand("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmd); err != nil {
		log.Printf("PowerShell hosts write failed: %v", err)
		return fmt.Errorf("failed to write hosts file with elevated permissions: %v", err)
	}

	// Force DNS cache flush
	go hm.flushDNSCache()

	return nil
}

// writeHostsWithRobocopy uses robocopy for system file operations
func (hm *HostsManager) writeHostsWithRobocopy(content string) error {
	// Create temporary file
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, "hosts_temp")

	if err := os.WriteFile(tempFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile)

	// Use robocopy to copy with system permissions
	hostsDir := filepath.Dir(WindowsHostsPath)
	_, err := hm.runCommand("robocopy", tempDir, hostsDir, "hosts_temp", "hosts", "/Y", "/R:3", "/W:1")

	// Robocopy exit codes 0-7 are considered success
	if err != nil {
		// Try icacls approach
		return hm.writeHostsWithIcacls(content)
	}

	go hm.flushDNSCache()
	return nil
}

// writeHostsWithIcacls uses icacls to modify permissions and write
func (hm *HostsManager) writeHostsWithIcacls(content string) error {
	log.Println("🔧 Trying icacls permission approach...")

	// First, try to take ownership and modify permissions
	_, err1 := hm.runCommand("takeown", "/f", WindowsHostsPath)
	_, err2 := hm.runCommand("icacls", WindowsHostsPath, "/grant", "Everyone:F")

	if err1 != nil || err2 != nil {
		log.Printf("⚠️ icacls permission change failed: %v, %v", err1, err2)
	}

	// Try direct write again after permission change
	if err := os.WriteFile(WindowsHostsPath, []byte(content), 0644); err == nil {
		// Restore permissions
		hm.runCommand("icacls", WindowsHostsPath, "/reset")
		go hm.flushDNSCache()
		return nil
	}

	return fmt.Errorf("icacls approach failed")
}

// cleanKidSafeSection removes any existing KidSafe section from hosts content
func (hm *HostsManager) cleanKidSafeSection(content string) string {
	lines := strings.Split(content, "\n")
	var cleanLines []string
	inKidSafeSection := false

	for _, line := range lines {
		if strings.Contains(line, "KidSafe PC Blocked Domains - START") {
			inKidSafeSection = true
			continue
		}
		if strings.Contains(line, "KidSafe PC Blocked Domains - END") {
			inKidSafeSection = false
			continue
		}
		if !inKidSafeSection {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

func (hm *HostsManager) updateHostsFile() error {
	// Start with current hosts content and clean any existing KidSafe section
	currentContent, err := hm.readHostsFile()
	if err != nil {
		log.Printf("Warning: Could not read current hosts file, using original: %v", err)
		currentContent = hm.originalHosts
	}

	// Clean existing KidSafe section from current content
	content := hm.cleanKidSafeSection(currentContent)

	// Add marker comments
	content += "\n\n# === KidSafe PC Blocked Domains - START ===\n"

	// Add blocked domains
	domainCount := 0
	for domain := range hm.blockedDomains {
		content += fmt.Sprintf("%s %s\n", BlockedIP, domain)
		domainCount++
		log.Printf("Adding to hosts: %s -> %s", domain, BlockedIP)
	}

	content += "# === KidSafe PC Blocked Domains - END ===\n"

	log.Printf("Updating hosts file with %d blocked domains", domainCount)

	if err := hm.writeHostsFile(content); err != nil {
		log.Printf("Failed to write hosts file: %v", err)
		return err
	}

	log.Println("Hosts file updated successfully")
	return nil
}

func (hm *HostsManager) createBackup() error {
	return hm.copyFile(WindowsHostsPath, hm.backupPath)
}

func (hm *HostsManager) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	scanner := bufio.NewScanner(srcFile)
	writer := bufio.NewWriter(dstFile)

	for scanner.Scan() {
		fmt.Fprintln(writer, scanner.Text())
	}

	writer.Flush()
	return scanner.Err()
}

func (hm *HostsManager) flushDNSCache() {
	log.Println("Flushing DNS cache to apply hosts file changes...")

	// Comprehensive DNS cache flush for Windows
	commands := [][]string{
		{"ipconfig", "/flushdns"},
		{"powershell", "-Command", "Clear-DnsClientCache"},
		{"powershell", "-Command", "Restart-Service -Name Dnscache -Force"},
		// Additional browser-specific cache clearing
		{"powershell", "-Command", "Get-Process chrome -ErrorAction SilentlyContinue | ForEach-Object { $_.CloseMainWindow() }; Start-Sleep 1"},
	}

	for _, cmd := range commands {
		if len(cmd) > 0 {
			exec := cmd[0]
			args := cmd[1:]
			if c := runCommand(exec, args...); c != nil {
				err := c.Run()
				if err != nil {
					log.Printf("Command failed (non-fatal): %v %v - %v", exec, args, err)
				} else {
					log.Printf("Successfully executed: %v %v", exec, args)
				}
			}
		}
	}

	log.Println("DNS cache flush completed")
}

// Cleanup removes KidSafe entries and restores original hosts file
func (hm *HostsManager) Cleanup() error {
	log.Println("Cleaning up hosts file modifications...")

	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	// Read current hosts file
	content, err := hm.readHostsFile()
	if err != nil {
		log.Printf("Warning: Could not read hosts file for cleanup: %v", err)
		return err
	}

	// Remove KidSafe sections
	lines := strings.Split(content, "\n")
	var cleanLines []string
	inKidSafeSection := false

	for _, line := range lines {
		if strings.Contains(line, "KidSafe PC Blocked Domains - START") {
			inKidSafeSection = true
			continue
		}
		if strings.Contains(line, "KidSafe PC Blocked Domains - END") {
			inKidSafeSection = false
			continue
		}
		if !inKidSafeSection {
			cleanLines = append(cleanLines, line)
		}
	}

	// Write cleaned content
	cleanContent := strings.Join(cleanLines, "\n")
	if err := hm.writeHostsFile(cleanContent); err != nil {
		log.Printf("Warning: Could not write cleaned hosts file: %v", err)
		return err
	}

	// Remove backup file if it exists
	if _, err := os.Stat(hm.backupPath); err == nil {
		os.Remove(hm.backupPath)
	}

	log.Println("Hosts file cleanup completed")
	return nil
}

// VerifyHostsFile checks if domains are actually in the hosts file
func (hm *HostsManager) VerifyHostsFile() (map[string]bool, error) {
	content, err := hm.readHostsFile()
	if err != nil {
		return nil, err
	}

	found := make(map[string]bool)
	lines := strings.Split(content, "\n")

	log.Println("=== Current Hosts File Content (KidSafe section) ===")
	inKidSafeSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "KidSafe PC Blocked Domains - START") {
			inKidSafeSection = true
			log.Println(line)
			continue
		}
		if strings.Contains(line, "KidSafe PC Blocked Domains - END") {
			inKidSafeSection = false
			log.Println(line)
			continue
		}

		if inKidSafeSection && line != "" && !strings.HasPrefix(line, "#") {
			log.Println(line)
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip := parts[0]
				domain := parts[1]
				found[domain] = (ip == BlockedIP)
			}
		}
	}

	log.Println("=== End Hosts File Content ===")
	return found, nil
}

// TestDomainBlocking tests if a domain resolves to our blocking IP
func (hm *HostsManager) TestDomainBlocking(domain string) bool {
	// Simple test by trying to resolve the domain
	// This will use the hosts file if it's working
	cmd := exec.Command("nslookup", domain)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	output, err := cmd.Output()
	if err != nil {
		log.Printf("nslookup failed for %s: %v", domain, err)
		return false
	}

	outputStr := string(output)
	log.Printf("nslookup %s result: %s", domain, outputStr)

	// Check if it resolves to our blocking IP
	return strings.Contains(outputStr, BlockedIP)
}

// Helper function to run commands (moved from other files)
func runCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// runCommand executes a system command and returns the output
func (hm *HostsManager) runCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Output()
}

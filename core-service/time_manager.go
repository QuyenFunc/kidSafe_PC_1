// core-service/time_manager.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// --- Structs để ánh xạ dữ liệu từ Firebase ---
type TimeSlot struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type DayRule struct {
	Enabled              bool       `json:"enabled"`
	DailyLimitMinutes    int        `json:"dailyLimitMinutes"`
	BreakIntervalMinutes int        `json:"breakIntervalMinutes"`
	BreakDurationMinutes int        `json:"breakDurationMinutes"`
	AllowedSlots         []TimeSlot `json:"allowedSlots"`
}

type TimeRules struct {
	Weekdays DayRule `json:"weekdays"`
	Weekends DayRule `json:"weekends"`
}

// Usage tracking struct
type UsageSession struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  int64     `json:"duration_minutes"`
}

type DailyUsage struct {
	Date     string         `json:"date"`
	Sessions []UsageSession `json:"sessions"`
	Total    int64          `json:"total_minutes"`
}

// --- TimeManager để quản lý trạng thái ---
type TimeManager struct {
	rules            *TimeRules
	isBlocked        bool
	isBreakTime      bool
	sessionStartTime time.Time
	lastBreakTime    time.Time
	dailyUsage       map[string]*DailyUsage // key: YYYY-MM-DD
	mutex            sync.RWMutex
	stopChan         chan bool
	ticker           *time.Ticker

	// Callback để thông báo status change
	onStatusChange func(blocked bool, reason string)

	// File paths for persistence
	usageDataFile string
}

// Firewall rule name constant
const FIREWALL_RULE_NAME = "KidSafe Time Block"

func NewTimeManager() *TimeManager {
	tm := &TimeManager{
		dailyUsage:    make(map[string]*DailyUsage),
		stopChan:      make(chan bool),
		usageDataFile: "./data/time_usage.json",
	}

	// Load existing usage data
	tm.loadUsageData()
	return tm
}

// --- Windows Firewall Functions ---

// Chặn mạng bằng cách thêm firewall rule
func (tm *TimeManager) blockNetwork() error {
	log.Println("🚫 Chặn truy cập internet...")

	// Xóa rule cũ trước (nếu có)
	tm.unblockNetwork()

	// Thêm rule chặn HTTP (port 80)
	cmd1 := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+FIREWALL_RULE_NAME+" HTTP",
		"dir=out",
		"action=block",
		"protocol=TCP",
		"localport=80")

	if err := cmd1.Run(); err != nil {
		log.Printf("❌ Lỗi khi chặn HTTP: %v", err)
		return err
	}

	// Thêm rule chặn HTTPS (port 443)
	cmd2 := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+FIREWALL_RULE_NAME+" HTTPS",
		"dir=out",
		"action=block",
		"protocol=TCP",
		"localport=443")

	if err := cmd2.Run(); err != nil {
		log.Printf("❌ Lỗi khi chặn HTTPS: %v", err)
		return err
	}

	tm.mutex.Lock()
	tm.isBlocked = true
	tm.mutex.Unlock()

	log.Println("✅ Đã chặn truy cập internet (HTTP/HTTPS)")
	return nil
}

// Mở lại mạng bằng cách xóa firewall rule
func (tm *TimeManager) unblockNetwork() error {
	log.Println("🔓 Mở lại truy cập internet...")

	// Xóa rule HTTP
	cmd1 := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name="+FIREWALL_RULE_NAME+" HTTP")
	cmd1.Run() // Không check error vì rule có thể không tồn tại

	// Xóa rule HTTPS
	cmd2 := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name="+FIREWALL_RULE_NAME+" HTTPS")
	cmd2.Run() // Không check error vì rule có thể không tồn tại

	tm.mutex.Lock()
	tm.isBlocked = false
	tm.mutex.Unlock()

	log.Println("✅ Đã mở lại truy cập internet")
	return nil
}

// Kiểm tra xem mạng có đang bị chặn không
func (tm *TimeManager) isNetworkBlocked() bool {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	return tm.isBlocked
}

// --- Usage Tracking Functions ---

// Lưu dữ liệu usage vào file
func (tm *TimeManager) saveUsageData() error {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	// Ensure data directory exists
	os.MkdirAll(filepath.Dir(tm.usageDataFile), 0755)

	data, err := json.MarshalIndent(tm.dailyUsage, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tm.usageDataFile, data, 0644)
}

// Load dữ liệu usage từ file
func (tm *TimeManager) loadUsageData() error {
	if _, err := os.Stat(tm.usageDataFile); os.IsNotExist(err) {
		return nil // File không tồn tại, skip
	}

	data, err := os.ReadFile(tm.usageDataFile)
	if err != nil {
		return err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	return json.Unmarshal(data, &tm.dailyUsage)
}

// Bắt đầu session sử dụng
func (tm *TimeManager) startSession() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.sessionStartTime = time.Now()
	log.Printf("⏱️ Bắt đầu session lúc: %s", tm.sessionStartTime.Format("15:04:05"))
}

// Kết thúc session và ghi nhận usage
func (tm *TimeManager) endSession() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if tm.sessionStartTime.IsZero() {
		return
	}

	now := time.Now()
	duration := now.Sub(tm.sessionStartTime)

	// Ghi nhận vào daily usage
	today := now.Format("2006-01-02")
	if tm.dailyUsage[today] == nil {
		tm.dailyUsage[today] = &DailyUsage{
			Date:     today,
			Sessions: []UsageSession{},
			Total:    0,
		}
	}

	session := UsageSession{
		StartTime: tm.sessionStartTime,
		EndTime:   now,
		Duration:  int64(duration.Minutes()),
	}

	tm.dailyUsage[today].Sessions = append(tm.dailyUsage[today].Sessions, session)
	tm.dailyUsage[today].Total += session.Duration

	log.Printf("⏱️ Kết thúc session: %d phút. Tổng hôm nay: %d phút",
		session.Duration, tm.dailyUsage[today].Total)

	// Reset session
	tm.sessionStartTime = time.Time{}

	// Save to file
	go tm.saveUsageData()
}

// Lấy total usage hôm nay
func (tm *TimeManager) getTodayUsage() int64 {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	today := time.Now().Format("2006-01-02")
	if usage, exists := tm.dailyUsage[today]; exists {
		return usage.Total
	}
	return 0
}

// --- Main Functions ---

// Cập nhật quy tắc mới từ Firebase
func (tm *TimeManager) UpdateRules(newRules TimeRules) {
	log.Println("📋 Cập nhật time rules từ Firebase")
	tm.mutex.Lock()
	tm.rules = &newRules
	tm.mutex.Unlock()

	// Trigger immediate check
	go tm.checkTimeRules()
}

// Lấy quy tắc hiện tại
func (tm *TimeManager) GetCurrentRules() *TimeRules {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	return tm.rules
}

// Set callback function
func (tm *TimeManager) SetStatusChangeCallback(callback func(blocked bool, reason string)) {
	tm.onStatusChange = callback
}

// Notify status change
func (tm *TimeManager) notifyStatusChange(blocked bool, reason string) {
	if tm.onStatusChange != nil {
		go tm.onStatusChange(blocked, reason)
	}
}

// Kiểm tra quy tắc thời gian
func (tm *TimeManager) checkTimeRules() {
	if tm.rules == nil {
		return
	}

	now := time.Now()
	today := now.Weekday()

	var currentRule DayRule
	var dayType string
	if today == time.Saturday || today == time.Sunday {
		currentRule = tm.rules.Weekends
		dayType = "Cuối tuần"
	} else {
		currentRule = tm.rules.Weekdays
		dayType = "Ngày thường"
	}

	if !currentRule.Enabled {
		// Rule disabled, unblock if blocked
		if tm.isNetworkBlocked() {
			tm.unblockNetwork()
			tm.notifyStatusChange(false, fmt.Sprintf("Quy tắc %s đã tắt", dayType))
		}
		return
	}

	// 1. Kiểm tra khung giờ cho phép
	isAllowedTime := tm.isInAllowedTimeSlot(currentRule.AllowedSlots, now)

	// 2. Kiểm tra giới hạn thời gian hàng ngày
	todayUsage := tm.getTodayUsage()
	isWithinDailyLimit := currentRule.DailyLimitMinutes == 0 || todayUsage < int64(currentRule.DailyLimitMinutes)

	// 3. Kiểm tra nghỉ ngơi bắt buộc
	needBreak := tm.needMandatoryBreak(currentRule)

	// Quyết định chặn hay mở
	shouldBlock := !isAllowedTime || !isWithinDailyLimit || needBreak

	// Log chi tiết
	var reason string
	if !isAllowedTime {
		reason = "Ngoài giờ cho phép"
	} else if !isWithinDailyLimit {
		reason = fmt.Sprintf("Đã vượt quá giới hạn %d phút/ngày (đã dùng %d phút)",
			currentRule.DailyLimitMinutes, todayUsage)
	} else if needBreak {
		reason = "Cần nghỉ ngơi bắt buộc"
	} else {
		reason = "Trong thời gian cho phép"
	}

	// Apply blocking/unblocking
	if shouldBlock && !tm.isNetworkBlocked() {
		tm.blockNetwork()
		tm.endSession() // End current session when blocked
		tm.notifyStatusChange(true, reason)
	} else if !shouldBlock && tm.isNetworkBlocked() {
		tm.unblockNetwork()
		tm.startSession() // Start new session when unblocked
		tm.notifyStatusChange(false, reason)
	}
}

// Kiểm tra xem có trong khung giờ cho phép không
func (tm *TimeManager) isInAllowedTimeSlot(slots []TimeSlot, now time.Time) bool {
	if len(slots) == 0 {
		return true // Không có giới hạn khung giờ
	}

	currentTime := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())

	for _, slot := range slots {
		if tm.isTimeInRange(currentTime, slot.StartTime, slot.EndTime) {
			return true
		}
	}
	return false
}

// Kiểm tra thời gian có trong khoảng không
func (tm *TimeManager) isTimeInRange(current, start, end string) bool {
	return current >= start && current <= end
}

// Kiểm tra cần nghỉ ngơi bắt buộc không
func (tm *TimeManager) needMandatoryBreak(rule DayRule) bool {
	if rule.BreakIntervalMinutes == 0 || rule.BreakDurationMinutes == 0 {
		return false // Không có yêu cầu nghỉ ngơi
	}

	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	// Nếu đang trong break time
	if tm.isBreakTime {
		// Kiểm tra đã đủ thời gian nghỉ chưa
		if time.Since(tm.lastBreakTime).Minutes() >= float64(rule.BreakDurationMinutes) {
			tm.isBreakTime = false
			log.Printf("✅ Kết thúc thời gian nghỉ ngơi")
			return false
		}
		return true // Vẫn đang trong thời gian nghỉ
	}

	// Kiểm tra có cần bắt đầu nghỉ không
	if !tm.sessionStartTime.IsZero() {
		sessionDuration := time.Since(tm.sessionStartTime).Minutes()
		if sessionDuration >= float64(rule.BreakIntervalMinutes) {
			tm.isBreakTime = true
			tm.lastBreakTime = time.Now()
			log.Printf("⏸️ Bắt đầu thời gian nghỉ ngơi bắt buộc (%d phút)", rule.BreakDurationMinutes)
			return true
		}
	}

	return false
}

// Vòng lặp chính để giám sát
func (tm *TimeManager) StartMonitoring() {
	log.Println("🕐 Bắt đầu dịch vụ quản lý thời gian")

	tm.ticker = time.NewTicker(30 * time.Second) // Kiểm tra mỗi 30 giây

	// Initial check
	go tm.checkTimeRules()

	for {
		select {
		case <-tm.ticker.C:
			tm.checkTimeRules()
		case <-tm.stopChan:
			log.Println("🛑 Dừng dịch vụ quản lý thời gian")
			return
		}
	}
}

// Dừng monitoring
func (tm *TimeManager) Stop() {
	log.Println("🛑 Đang dừng TimeManager...")

	if tm.ticker != nil {
		tm.ticker.Stop()
	}

	// End current session
	tm.endSession()

	// Unblock network
	tm.unblockNetwork()

	// Signal stop
	close(tm.stopChan)

	log.Println("✅ TimeManager đã dừng")
}

// Lấy status hiện tại
func (tm *TimeManager) GetStatus() map[string]interface{} {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	status := map[string]interface{}{
		"is_blocked":    tm.isBlocked,
		"is_break_time": tm.isBreakTime,
		"today_usage":   tm.getTodayUsage(),
		"has_rules":     tm.rules != nil,
	}

	if tm.rules != nil {
		now := time.Now()
		var currentRule DayRule
		if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
			currentRule = tm.rules.Weekends
		} else {
			currentRule = tm.rules.Weekdays
		}

		status["current_rule"] = currentRule
		status["daily_limit"] = currentRule.DailyLimitMinutes
	}

	if !tm.sessionStartTime.IsZero() {
		status["session_duration"] = int64(time.Since(tm.sessionStartTime).Minutes())
	}

	return status
}

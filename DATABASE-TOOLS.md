# KidSafe PC Database Tools

Các công cụ tiện ích để quản lý database của KidSafe PC.

## 📁 Files

- `core-service/db-utils.go` - Utility functions cho database operations
- `db-tools.ps1` - PowerShell script để sử dụng các utility functions
- `restart-service.ps1` - Script để restart KidSafe PC service

## 🛠️ Cách sử dụng

### 1. Kiểm tra database contents

```powershell
.\db-tools.ps1 check
```

Hiển thị tất cả blocking rules trong database với:
- ✅/❌ Status (active/inactive)
- 📱 Firebase sync rules 
- 🌐 Other rules
- Thống kê tổng quan

### 2. Sync Firebase URLs

```powershell
.\db-tools.ps1 sync
```

Thêm sample Firebase URLs vào database:
- facebook.com
- instagram.com
- tiktok.com
- youtube.com

### 3. Restart service

```powershell
.\restart-service.ps1
```

Restart KidSafe PC service với phiên bản mới.

### 4. Help

```powershell
.\db-tools.ps1 help
```

Hiển thị hướng dẫn sử dụng.

## 🔧 Technical Details

### Database Schema

```sql
CREATE TABLE block_rules (
    id INTEGER PRIMARY KEY,
    domain TEXT NOT NULL,
    category TEXT NOT NULL,
    profile_id INTEGER,
    reason TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT 1
);
```

### Categories

- `firebase-sync` - URLs synced từ Android app
- `other` - URLs được thêm từ PC UI hoặc AI suggestions

### Utility Functions

- `CheckDatabaseContents()` - Hiển thị tất cả rules
- `SyncSampleFirebaseUrls()` - Thêm sample Firebase URLs
- `TestDatabaseAPI()` - Test API logic

## 🚀 Workflow

1. **Thêm URLs từ Android** → Firebase → PC database (automatic)
2. **Manual sync** → `.\db-tools.ps1 sync`
3. **Kiểm tra database** → `.\db-tools.ps1 check`
4. **Restart service** → `.\restart-service.ps1`
5. **Mở PC UI** → Xem URLs trong "Blocked Websites"

## ⚠️ Lưu ý

- Chạy PowerShell với quyền Administrator khi cần
- Service cần restart sau khi thay đổi code
- Database path: `core-service/data/parental_control.db`
- API endpoint: `http://127.0.0.1:8081/api/v1/rules`

## 🎯 Troubleshooting

**API trả về null:**
1. Kiểm tra database: `.\db-tools.ps1 check`
2. Restart service: `.\restart-service.ps1`
3. Test API: `curl http://127.0.0.1:8081/api/v1/rules`

**Firebase sync không hoạt động:**
1. Manual sync: `.\db-tools.ps1 sync`
2. Kiểm tra Firebase credentials
3. Restart service

**Build errors:**
1. Xóa các file temp: `temp-*.go`
2. Build lại: `go build -o kidsafe-pc.exe .`

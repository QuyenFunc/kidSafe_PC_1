# KidSafe PC Browser Extension

## 📋 Mô tả

Extension trình duyệt bổ sung cho ứng dụng **KidSafe PC**, hiển thị trang thông báo thân thiện và giáo dục khi trẻ em cố gắng truy cập các trang web bị chặn.

## 🎯 Tính năng

### 🛡️ Bảo vệ thông minh
- Tự động phát hiện khi trang web bị chặn bởi hosts file
- Thay thế trang lỗi khó hiểu bằng thông báo thân thiện
- Giải thích cho trẻ em tại sao trang web bị chặn

### 🎨 Giao diện thân thiện
- Thiết kế đẹp mắt, phù hợp với trẻ em
- Sử dụng emoji và màu sắc tươi sáng
- Thông điệp tích cực và giáo dục

### 📊 Thống kê và theo dõi
- Đếm số lượng trang web đã chặn
- Hiển thị thời gian chặn gần nhất
- Badge notification trên icon extension

## 🚀 Cài đặt

### Phương pháp 1: Tự động (Khuyên dùng)
1. Chạy file `install-extension.bat` với quyền Administrator
2. Làm theo hướng dẫn hiển thị

### Phương pháp 2: Thủ công
1. Mở Google Chrome
2. Vào `chrome://extensions/`
3. Bật "Developer mode" ở góc trên bên phải
4. Click "Load unpacked"
5. Chọn thư mục `kidsafe-extension`
6. Extension sẽ xuất hiện trong danh sách

## 🔧 Cách hoạt động

### Phát hiện trang bị chặn
Extension sử dụng nhiều phương pháp để phát hiện trang web bị chặn:

- **URL Analysis**: Kiểm tra hostname (127.0.0.1, localhost)
- **Page Title**: Phân tích tiêu đề trang (ERR_CONNECTION_REFUSED, etc.)
- **Content Analysis**: Tìm kiếm nội dung lỗi Chrome
- **Error Codes**: Nhận diện mã lỗi DNS và kết nối

### Hiển thị trang thông báo
Khi phát hiện trang bị chặn, extension sẽ:

1. Thay thế toàn bộ nội dung trang
2. Hiển thị thông báo thân thiện với:
   - Icon bảo vệ và emoji dễ thương
   - Giải thích tại sao trang bị chặn
   - Gợi ý hoạt động thay thế tích cực
   - Nút quay về trang an toàn

## 🎨 Tùy chỉnh

### Thay đổi thông điệp
Chỉnh sửa file `content.js` trong phần `blockedPageHTML` để:
- Thay đổi nội dung thông báo
- Điều chỉnh màu sắc và styling
- Thêm emoji hoặc hình ảnh mới

### Cập nhật logic phát hiện
Trong file `content.js`, mảng `BLOCKED_INDICATORS` chứa các điều kiện phát hiện trang bị chặn. Bạn có thể:
- Thêm điều kiện phát hiện mới
- Điều chỉnh độ nhạy
- Loại trừ các trang cụ thể

## 📱 Tương thích

### Trình duyệt được hỗ trợ
- ✅ **Google Chrome** (phiên bản 88+)
- ✅ **Microsoft Edge** (Chromium-based)
- ✅ **Brave Browser**
- ✅ **Opera** (Chromium-based)

### Hệ điều hành
- ✅ **Windows 10/11**
- ✅ **macOS** (cần điều chỉnh đường dẫn hosts)
- ✅ **Linux** (cần điều chỉnh đường dẫn hosts)

## 🔒 Bảo mật và Quyền riêng tư

### Quyền được yêu cầu
- `activeTab`: Truy cập tab hiện tại để phát hiện trang bị chặn
- `storage`: Lưu trữ thống kê và cấu hình
- `webNavigation`: Theo dõi điều hướng để phát hiện lỗi
- `<all_urls>`: Hoạt động trên tất cả trang web

### Dữ liệu thu thập
Extension chỉ lưu trữ cục bộ:
- Số lượng trang đã chặn
- Thời gian chặn gần nhất
- Domain của trang bị chặn (không lưu URL đầy đủ)

**⚠️ Không gửi dữ liệu ra ngoài!**

## 🛠️ Phát triển

### Cấu trúc thư mục
```
kidsafe-extension/
├── manifest.json          # Cấu hình extension
├── content.js             # Script chính phát hiện và hiển thị
├── background.js          # Background service worker
├── popup.html            # Giao diện popup
├── popup.js              # Logic popup
├── assets/               # Icons và hình ảnh
├── install-extension.bat # Script cài đặt
└── README.md            # Tài liệu này
```

### Debugging
1. Mở Chrome DevTools
2. Vào tab "Sources"
3. Tìm "Content Scripts" để debug content.js
4. Kiểm tra Console để xem log

### Testing
- Test với URL: `http://127.0.0.1/test`
- Kiểm tra trên trang có lỗi ERR_CONNECTION_REFUSED
- Verify popup hiển thị thống kê chính xác

## 🤝 Đóng góp

Để đóng góp vào project:

1. Fork repository
2. Tạo feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Tạo Pull Request

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.

## 📞 Hỗ trợ

Nếu gặp vấn đề:

1. Kiểm tra Extension có được bật không
2. Verify ứng dụng KidSafe PC đang chạy
3. Khởi động lại trình duyệt
4. Kiểm tra Console cho error messages

---

**Made with ❤️ for protecting children online**

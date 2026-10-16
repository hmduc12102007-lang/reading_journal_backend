# Rà soát backend — 16/10/2026

## Đã bổ sung và sửa

- Upload, xem và xóa ảnh bìa qua `/api/v1/books/:book_id/cover`. Tạo sách trước, rồi upload bằng `book_id`.
- File ảnh lưu trong `UPLOAD_DIR`; PostgreSQL lưu URL và tên file nội bộ. AutoMigrate thêm `books.cover_file`, không cần bảng mới.
- Chỉ người tạo sách được thay ảnh. Quyền xem ảnh giống quyền xem sách; sách riêng tư cần Bearer token.
- Kiểm tra nội dung JPEG/PNG/GIF, ảnh hỏng, dung lượng tối đa 5 MiB và kích thước tối đa 20 triệu pixel. Tên file do server sinh ngẫu nhiên. Không mở thư mục upload dưới dạng static công khai.
- Xóa file cũ khi thay/xóa ảnh hoặc thay URL ảnh; gỡ file mới nếu cập nhật database thất bại.
- Kiểm tra tên sách, tác giả, độ dài publisher/ISBN/URL, giao thức URL ảnh và tên mục tiêu chỉ chứa khoảng trắng.
- Chặn phân trang gây tràn số và ID vượt phạm vi bigint của PostgreSQL.
- Sửa tiến độ sách hoàn thành: hoàn thành session đưa trang hiện tại về tổng số trang; thư viện hiện 100%, đọc lại bắt đầu session mới ở trang 0.
- Đồng nhất thứ tự khóa thư viện rồi session khi hoàn thành, tránh thứ tự khóa ngược với thao tác bắt đầu đọc.
- Phân biệt lỗi truy vấn database và không tìm thấy dữ liệu ở các luồng đã sửa; ID sai trong API ghi chú/đánh giá trả 400.
- Bắt buộc token JWT có hạn sử dụng. Thêm giới hạn body HTTP 6 MiB, timeout kết nối và thông báo lỗi HTTP tương ứng.
- Cập nhật README và hợp đồng Apidog để gọi được các API mới.

## Kiểm chứng

`go test ./... -count=1`, bao gồm kiểm thử tích hợp với PostgreSQL 18 trong một cluster thử riêng trên cổng 55439, đã thành công. Kiểm thử tạo schema ngẫu nhiên, chạy AutoMigrate hai lần, thực hiện các luồng API và dọn schema. `go vet ./...` và `git diff --check` cũng thành công.

Các luồng được kiểm tra gồm đăng ký trùng email, sai mật khẩu, truy cập chéo tài khoản, upload ảnh giả/quá lớn, tải ảnh riêng tư, thay/xóa ảnh và dọn file, thay URL, tiến độ/hoàn thành/đọc lại, ghi chú, đánh giá, mục tiêu và vô hiệu hóa token sau đổi mật khẩu. Kiểm thử HTTP trong tiến trình Go xác nhận body quá lớn trả 413 và thông báo lỗi không lộ chi tiết nội bộ.

Kiểm chứng này không thực hiện migration hay sửa dữ liệu trong database ứng dụng hiện có. Chưa kiểm thử trực tiếp với ứng dụng Flutter hay môi trường triển khai thực tế.

## Phần cần cấu hình khi chạy ứng dụng

1. Workspace hiện chưa có `.env`. Sao chép `.env.example`, điền `DB_CONNECTION_STRING` đúng tài khoản PostgreSQL và `JWT_SECRET` ngẫu nhiên tối thiểu 32 ký tự. Không commit `.env`.
2. Go đã có tại `C:/Users/ASUS/sdk/go1.27.2/bin`, nhưng lệnh `go` chưa nằm trong PATH của phiên terminal này. Cấu hình PATH hoặc dùng Go SDK trong GoLand.
3. Flutter cần gửi multipart field `image`, phân biệt `book_id` với `user_book_id`, nối URL tương đối với địa chỉ server và gửi Bearer token khi xem ảnh riêng tư. Chỉ gửi token tới backend của ứng dụng.
4. Khi triển khai, `UPLOAD_DIR` cần ổ đĩa bền vững có quyền ghi. Sao lưu cả PostgreSQL lẫn thư mục ảnh; nếu chạy nhiều instance, dùng cùng vùng lưu trữ. Hiện chưa tích hợp dịch vụ lưu trữ đám mây.

Ảnh đại diện người dùng nằm ngoài đặc tả hiện tại. Các chức năng đổi email, quên mật khẩu, refresh token, xóa tài khoản, xóa lịch sử đọc và quản trị sách dùng chung cũng chưa có trong hợp đồng API; cần chốt quy tắc nghiệp vụ trước khi bổ sung. Đây là các hướng mở rộng, không được coi là đã triển khai trong lần sửa này.

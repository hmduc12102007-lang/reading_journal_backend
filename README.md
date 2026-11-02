# Reading Journal Backend

Backend cho đồ án **Ứng dụng quản lý sách và nhật ký đọc sách Reading Journal**, sử dụng Go, Echo, GORM và PostgreSQL. Ứng dụng Flutter kết nối với backend qua REST API; mã nguồn frontend được bàn giao riêng.

## Chức năng

- Đăng ký, đăng nhập bằng JWT, quản lý hồ sơ và đổi mật khẩu.
- Quản lý sách, thư viện cá nhân, yêu thích và ảnh bìa.
- Theo dõi tiến độ, hoàn thành và đọc lại, giữ lịch sử theo từng lượt đọc.
- Quản lý nhật ký, ghi chú và đánh giá sách.
- Thiết lập mục tiêu theo số sách hoặc số trang; xem tổng quan và thống kê.
- Kiểm tra quyền truy cập dữ liệu theo tài khoản.

## Công nghệ và cấu trúc

- Go từ 1.23; Echo v4; GORM; PostgreSQL.
- JWT để xác thực, bcrypt để băm mật khẩu.
- Ảnh bìa lưu trên hệ thống tệp của máy chủ.

```text
reading_journal_backend/
├── cmd/server/         # Khởi động server và cấu hình HTTP
├── internal/
│   ├── auth/           # JWT và middleware xác thực
│   ├── config/         # Đọc cấu hình môi trường
│   ├── database/       # Kết nối và migration PostgreSQL
│   ├── handlers/       # Xử lý API và nghiệp vụ
│   ├── models/         # Mô hình dữ liệu
│   └── routes/         # Khai báo endpoint
├── .env.example        # Cấu hình mẫu, không chứa bí mật
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

## Cài đặt và chạy

### 1. Chuẩn bị cơ sở dữ liệu

Cài Go và PostgreSQL. Trong pgAdmin hoặc psql, tạo cơ sở dữ liệu bằng tài khoản có quyền phù hợp:

```sql
CREATE DATABASE reading_journal;
```

### 2. Cấu hình môi trường

Mở PowerShell tại thư mục dự án, sao chép cấu hình mẫu:

```powershell
Copy-Item .env.example .env
```

Điền các biến trong `.env`:

| Biến | Ý nghĩa |
| --- | --- |
| `APP_ENV` | Tên môi trường; mặc định `development`. |
| `APP_PORT` | Cổng HTTP; mặc định `8080`. |
| `DB_CONNECTION_STRING` | URL PostgreSQL: `postgres://USER:PASSWORD@localhost:5432/reading_journal?sslmode=disable`. Thay USER và PASSWORD bằng tài khoản trên máy. |
| `JWT_SECRET` | Chuỗi bí mật ngẫu nhiên, tối thiểu 32 ký tự; phải điền trước khi chạy. |
| `UPLOAD_DIR` | Thư mục ảnh bìa; mặc định `uploads/covers`. |

Không đưa `.env` thật lên GitHub. `.env.example` được giữ để người nhận mã nguồn biết các biến cần cấu hình. `sslmode=disable` trong URL trên dành cho môi trường phát triển cục bộ.

### 3. Khởi chạy backend

```powershell
go mod download
go run ./cmd/server
```

Server mặc định chạy tại `http://localhost:8080`. Khi khởi động, GORM AutoMigrate tạo hoặc cập nhật tám bảng nghiệp vụ. Nên dùng cơ sở dữ liệu mới khi chạy lần đầu; sao lưu trước nếu sử dụng dữ liệu cũ.

Trong cửa sổ PowerShell khác, kiểm tra phản hồi HTTP:

```powershell
Invoke-RestMethod http://localhost:8080/health
```

Kết quả mong đợi là `success: true` và `data.status: ok`. Endpoint này kiểm tra HTTP, không thay thế kiểm thử đọc và ghi dữ liệu.

### 4. Kết nối ứng dụng Flutter

| Môi trường | Địa chỉ API |
| --- | --- |
| Máy đang chạy backend | `http://localhost:8080/api/v1` |
| Android Emulator | `http://10.0.2.2:8080/api/v1` |
| Điện thoại thật | `http://<IP_LAN_MAY_CHU>:8080/api/v1` |

Điện thoại và máy chủ cần kết nối được qua mạng; cho phép cổng backend qua tường lửa khi thử trên thiết bị thật. Khi triển khai công khai, cấu hình HTTPS và lưu bền vững cả PostgreSQL lẫn thư mục ảnh.

## API

Các endpoint nghiệp vụ có tiền tố `/api/v1`. Endpoint `GET /health` nằm ngoài tiền tố này. Danh sách route được khai báo tại [internal/routes/routes.go](internal/routes/routes.go).

| Nhóm | Phương thức và đường dẫn dưới `/api/v1` |
| --- | --- |
| Đăng ký và đăng nhập | `POST /auth/register`, `POST /auth/login` |
| Mật khẩu và hồ sơ | `PATCH /auth/change-password`, `GET /users/me`, `PATCH /users/me` |
| Sách | `GET /books`, `POST /books`, `GET /books/{book_id}`, `PATCH /books/{book_id}` |
| Ảnh bìa | `POST /books/{book_id}/cover`, `GET /books/{book_id}/cover`, `DELETE /books/{book_id}/cover` |
| Thư viện | `GET /me/books`, `POST /me/books`, `POST /me/books/new` |
| Sách trong thư viện | `GET /me/books/{user_book_id}`, `PATCH /me/books/{user_book_id}`, `DELETE /me/books/{user_book_id}` |
| Lượt đọc | `GET /me/books/{user_book_id}/sessions`, `POST /me/books/{user_book_id}/sessions` |
| Tiến độ và hoàn thành | `PATCH /sessions/{session_id}/progress`, `POST /sessions/{session_id}/complete` |
| Nhật ký | `GET /reading-logs`, `POST /reading-logs`, `GET /reading-logs/{log_id}`, `PATCH /reading-logs/{log_id}`, `DELETE /reading-logs/{log_id}` |
| Ghi chú | `GET /me/books/{user_book_id}/notes`, `POST /me/books/{user_book_id}/notes`, `PATCH /notes/{note_id}`, `DELETE /notes/{note_id}` |
| Đánh giá | `GET /me/books/{user_book_id}/review`, `PUT /me/books/{user_book_id}/review`, `DELETE /me/books/{user_book_id}/review` |
| Mục tiêu | `GET /goals`, `POST /goals`, `GET /goals/{goal_id}`, `PATCH /goals/{goal_id}`, `DELETE /goals/{goal_id}` |
| Tổng quan và thống kê | `GET /dashboard`, `GET /statistics` |

API dữ liệu cá nhân yêu cầu header `Authorization: Bearer <access_token>`. Đăng ký và đăng nhập không cần token. Tra cứu sách chung và ảnh sách chung cho phép khách; sách riêng yêu cầu quyền của người tạo.

JSON sử dụng `snake_case`; ngày nghiệp vụ có dạng `YYYY-MM-DD`. Phản hồi thành công có `success` và `data`; phản hồi lỗi có `success: false`, `error.code` và `error.message`. Danh sách có phân trang trả thêm `pagination`. Phản hồi `204 No Content` không có body.

### Quy tắc nghiệp vụ

- `books` lưu thông tin sách; `user_books` liên kết sách với thư viện của từng tài khoản. Mỗi cặp tài khoản và sách chỉ xuất hiện một lần.
- `POST /books` chỉ tạo sách. `POST /me/books` thêm sách có sẵn. `POST /me/books/new` tạo sách và thêm vào thư viện trong cùng transaction.
- Sách người dùng tạo là sách riêng; sách chung có `created_by` rỗng. `category_name` là thuộc tính sách, chưa có chức năng quản lý danh mục thể loại riêng.
- Không bỏ sách khỏi thư viện khi đã có lượt đọc, ghi chú hoặc đánh giá; API trả `409 BOOK_HAS_HISTORY`.
- Cập nhật trang và ghi nhật ký là hai thao tác riêng. Nhật ký không tự thay đổi `current_page`.
- Cập nhật đến trang cuối không tự hoàn thành. Hoàn thành là thao tác riêng; đọc lại tạo lượt mới và giữ lịch sử cũ.
- Mục tiêu `BOOKS` đếm các `book_id` khác nhau hoàn thành trong kỳ. `PAGES` cộng số trang trong nhật ký, tính cả khoảng trùng và đọc lại.
- Đổi mật khẩu làm mất hiệu lực các token cũ của tài khoản.

### Ảnh bìa

Upload dùng `book_id`, gửi `multipart/form-data` với đúng một file tên `image`. Backend nhận JPEG, PNG hoặc GIF hợp lệ, tối đa 5 MiB và 20 triệu pixel; giới hạn tổng request là 6 MiB.

Ảnh riêng được truy xuất qua API có kiểm tra quyền. Flutter cần nối `cover_url` tương đối với địa chỉ gốc máy chủ và gửi token khi tải ảnh riêng. Chỉ gửi token đến backend của ứng dụng, không gửi đến URL ảnh bên ngoài.

## Kiểm thử và biên dịch

Chạy các lệnh tại thư mục dự án:

```powershell
go test ./... -count=1 -v
go vet ./...
go build -o reading-journal.exe ./cmd/server
```

Các kiểm thử tự động được giữ cùng mã nguồn. Để chạy `TestAPIIntegration`, cấu hình biến môi trường `TEST_DATABASE_URL` bằng URL PostgreSQL của môi trường thử nghiệm, rồi chạy lại lệnh kiểm thử. Tài khoản thử nghiệm cần quyền tạo schema.

Test tích hợp tạo schema ngẫu nhiên và dọn schema đó sau khi chạy. Nếu không có `TEST_DATABASE_URL`, test này được **SKIP**; kết quả đó không xác nhận luồng tích hợp PostgreSQL.

## Phạm vi bàn giao

Kho này chứa backend và kiểm thử. Frontend Flutter do thành viên phụ trách bàn giao riêng. Các chức năng khôi phục mật khẩu, xác minh email, refresh token, ảnh đại diện và đồng bộ ngoại tuyến chưa được triển khai.

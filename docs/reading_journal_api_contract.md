# TÀI LIỆU THỐNG NHẤT NGHIỆP VỤ & API

**Dự án:** Reading Journal App – Ứng dụng quản lý sách và nhật ký đọc sách  
**Frontend:** Flutter (Dart) · **Backend:** Go + Echo + GORM · **Database:** PostgreSQL  
**Phiên bản:** 1.0 – Hợp đồng API cho Frontend và Backend  
**Phạm vi:** 8 bảng nghiệp vụ; `category_name` là tên tự nhập trên từng sách, không quản lý bảng thể loại riêng; **không có ảnh đại diện người dùng**; có ảnh bìa sách.

> **Lưu ý:** Tài liệu này dựa trên mô tả chức năng và schema SQL đã trao đổi, chưa đối chiếu trực tiếp toàn bộ source Flutter mới nhất. Những điểm cần quyết định được ghi rõ ở cuối; không xem các API bên dưới là đã triển khai.

---

## 1. Mục tiêu và nguyên tắc chung

- Flutter chỉ quản lý giao diện, nhập liệu, gọi API, hiển thị dữ liệu và giữ một ít cài đặt cục bộ.
- Backend kiểm tra dữ liệu, xác thực tài khoản, kiểm tra quyền sở hữu, xử lý nghiệp vụ và truy vấn PostgreSQL.
- PostgreSQL là nguồn dữ liệu chính của sách, thư viện cá nhân, lượt đọc, nhật ký, ghi chú, đánh giá và mục tiêu.
- Không lưu mật khẩu dạng rõ; `password_hash` chỉ được tạo và sử dụng trong Backend.
- Không tin `user_id` do Flutter gửi trong body. Backend xác định người dùng từ token đăng nhập.
- Các thao tác thay đổi nhiều bảng phải dùng transaction để tránh dữ liệu lệch nhau.
- Thống nhất dùng tên trường **snake_case** trong JSON và Database, trạng thái viết HOA như các giá trị quy định.

### 1.1 Luồng dữ liệu

`Flutter → HTTP request (JSON) → Echo route → middleware xác thực → handler/service → PostgreSQL → JSON response → Flutter`

### 1.2 Cấu hình API

- Base URL: `/api/v1`.
- Khi chạy cùng máy: `http://localhost:8080/api/v1`.
- **Android Emulator** thường truy cập máy chủ trên laptop qua `http://10.0.2.2:8080/api/v1`, không phải `localhost`.
- **Thiết bị thật** dùng IP LAN của laptop, cần cùng mạng và cấu hình firewall phù hợp; khi triển khai thực tế dùng HTTPS.
- Header khi đăng nhập: `Authorization: Bearer <access_token>`.
- Request/response: `Content-Type: application/json; charset=utf-8`.
- Thời gian trả về dùng ISO 8601; ngày không có giờ: `YYYY-MM-DD`.
- ID do Database sinh; Flutter không tự tạo `user_id`, `book_id`, `session_id`…

---

## 2. Danh sách 8 bảng Database

| Bảng | Vai trò | Các cột chính |
|---|---|---|
| `users` | Tài khoản | `user_id` PK, `username`, `email` UNIQUE, `password_hash`, `created_at`, `updated_at` |
| `books` | Thông tin sách | `book_id` PK, `category_name` tùy chọn (chuỗi), `title`, `author`, `description`, `total_pages`, `cover_url`, `publisher`, `published_year`, `isbn`, `created_by` FK, `created_at` |
| `user_books` | Sách trong thư viện cá nhân | `user_book_id` PK, `user_id` FK, `book_id` FK, `status`, `is_favorite`, `added_at`; UNIQUE(`user_id`,`book_id`) |
| `reading_sessions` | Mỗi lần đọc/đọc lại | `session_id` PK, `user_book_id` FK, `session_number`, `start_date`, `end_date`, `current_page`, `status` |
| `reading_logs` | Nhật ký từng lần đọc | `log_id` PK, `session_id` FK, `read_date`, `start_page`, `end_page`, `content`, `created_at`, `updated_at` |
| `book_notes` | Ghi chú riêng | `note_id` PK, `user_book_id` FK, `page_number`, `content`, `created_at`, `updated_at` |
| `reviews` | Đánh giá cá nhân | `review_id` PK, `user_book_id` FK UNIQUE, `rating`, `comment`, `created_at`, `updated_at` |
| `reading_goals` | Mục tiêu đọc | `goal_id` PK, `user_id` FK, `goal_name`, `goal_type`, `target_value`, `start_date`, `end_date`, `created_at` |

**Quan hệ:** `users 1–N user_books`; `books 1–N user_books`; `user_books 1–N reading_sessions`; `reading_sessions 1–N reading_logs`; `user_books 1–N book_notes`; `user_books 1–0..1 reviews`; `users 1–N reading_goals`. `category_name` không phải khóa ngoại và không có danh sách category dùng chung.

**Lưu ý về schema hiện tại:** Không có trigger/index tự tạo riêng. `updated_at` phải được Backend cập nhật trong câu lệnh `UPDATE`. Schema hiện tại chưa tự bảo đảm mọi quy tắc liên bảng như `current_page <= total_pages` hoặc chỉ một lượt `READING` cho mỗi sách; Backend phải kiểm tra, và có thể bổ sung ràng buộc sau.

---

## 3. Quy tắc nghiệp vụ thống nhất

### NV-01. Tài khoản

1. Người dùng đăng ký bằng `username`, `email`, `password`.
2. Email không được trùng; Backend chuẩn hóa email và băm mật khẩu trước khi lưu.
3. Đăng nhập trả về `access_token` và thông tin tài khoản **không có** `password_hash`.
4. Mỗi người chỉ xem, sửa, xóa sách trong thư viện, nhật ký, mục tiêu, ghi chú và đánh giá của chính mình.
5. Không có upload ảnh đại diện.

### NV-02. Thêm sách vào thư viện

1. Người dùng chọn sách có sẵn hoặc tự nhập sách mới.
2. Nếu sách mới, Backend tạo bản ghi `books` (nếu hợp lệ) rồi tạo `user_books` cho người dùng trong **một transaction**.
3. `user_books` mặc định `WANT_TO_READ`, `is_favorite=false`.
4. Không được thêm trùng cùng `book_id` vào thư viện của cùng một người.
5. Ảnh bìa: lưu **đường dẫn** `cover_url`, không lưu file ảnh nhị phân trong bảng `books`. Việc upload file ảnh là chức năng riêng, chưa nằm trong schema.
6. **Đề xuất quyền riêng tư:** sách do người dùng tạo (`created_by`) chỉ chủ sở hữu sửa/xóa; cần chốt sách này có hiển thị trong tìm kiếm toàn hệ thống không.

### NV-03. Quản lý trạng thái sách

- `WANT_TO_READ`: chưa bắt đầu đọc.
- `READING`: có lượt đọc đang diễn ra.
- `COMPLETED`: lượt đọc gần nhất đã hoàn thành.
- Chuyển sang `READING`: tạo hoặc tiếp tục `reading_sessions` phù hợp.
- Hoàn thành: cập nhật lượt đọc thành `COMPLETED`, đặt `end_date`; đồng bộ `user_books.status`.
- Không cho cập nhật `status` một cách độc lập khiến trạng thái thư viện khác lượt đọc.

### NV-04. Tiến độ và đọc lại

1. Bắt đầu đọc tạo `reading_sessions` với `session_number=1`, `current_page=0`, `status=READING`.
2. Cập nhật tiến độ phải thỏa `0 <= current_page <= books.total_pages`.
3. Chỉ được có **một lượt đang đọc** trên một `user_book_id`.
4. Khi đọc lại sau khi đã hoàn thành, tạo lượt mới với `session_number` tăng thêm 1; **không xóa nhật ký lượt cũ**.
5. Không tự động xem một lượt là hoàn thành chỉ vì người dùng nhập trang cuối; nên có thao tác “Hoàn thành” rõ ràng.
6. Mọi thay đổi trạng thái lượt đọc và `user_books` cần transaction.

### NV-05. Nhật ký đọc

1. Nhật ký thuộc một `reading_session` cụ thể.
2. Các trường bắt buộc: `session_id`, `read_date`, `start_page`, `end_page`; `content` tùy chọn.
3. `1 <= start_page <= end_page <= books.total_pages`.
4. Cho phép nhiều nhật ký trong một ngày; mỗi bản ghi có `log_id` riêng.
5. Khi sửa/xóa nhật ký, Backend phải kiểm tra chủ sở hữu qua `reading_sessions → user_books → users`.
6. **Quy tắc đề xuất:** `current_page` được cập nhật bằng thao tác tiến độ riêng; tạo nhật ký không tự thay đổi tiến độ nếu hai bên chưa thống nhất.

### NV-06. Ghi chú và đánh giá

- Ghi chú (`book_notes`) thuộc sách trong thư viện cá nhân, có thể có `page_number`; nếu nhập phải nằm trong khoảng `1..total_pages`.
- Đánh giá (`reviews`) mỗi `user_book_id` tối đa một bản ghi, `rating` từ 1 đến 5; chỉnh sửa thì cập nhật đánh giá cũ.

### NV-07. Mục tiêu đọc

- `goal_type=BOOKS` hoặc `PAGES`, `target_value > 0`, `end_date >= start_date`.
- Mục tiêu `BOOKS`: đề xuất đếm **sách duy nhất** hoàn thành trong khoảng thời gian, không đếm lượt đọc lại.
- Mục tiêu `PAGES`: cần chốt cách tính từ nhật ký khi có khoảng trang trùng và đọc lại; xem mục 8.
- Tiến độ mục tiêu tính khi truy vấn, không cần cột `progress` trong Database.

### NV-08. Xóa dữ liệu

- Schema hiện tại dùng FK mặc định `NO ACTION` (không có `ON DELETE CASCADE`).
- **Đề xuất MVP:** xóa sách khỏi thư viện chỉ khi chưa có lượt đọc, ghi chú hoặc đánh giá; nếu đã có dữ liệu liên quan, trả `409 CONFLICT` và yêu cầu xác nhận chính sách xóa.
- Không xóa trực tiếp bản ghi `books` đang được người khác sử dụng.
- Nếu quyết định cho phép xóa toàn bộ lịch sử, Backend phải xử lý các bản ghi phụ thuộc trong transaction và xác nhận rõ ở Frontend.

---

## 4. Chuẩn request và response

### 4.1 Thành công

```json
{
  "success": true,
  "message": "Thao tác thành công",
  "data": { "id": 1 }
}
```

Với danh sách:

```json
{
  "success": true,
  "data": [
    { "book_id": 1, "title": "Sách mẫu" }
  ],
  "pagination": { "page": 1, "limit": 20, "total": 1 }
}
```

### 4.2 Thất bại

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Số trang không hợp lệ",
    "details": { "current_page": "Phải nằm trong khoảng 0..total_pages" }
  }
}
```

| HTTP | Ý nghĩa | Ví dụ |
|---|---|---|
| `200` | Đọc/cập nhật thành công | GET, PATCH |
| `201` | Tạo thành công | POST |
| `204` | Xóa thành công, không có body | DELETE |
| `400` | Dữ liệu đầu vào không hợp lệ | Trang âm, ngày sai |
| `401` | Chưa đăng nhập/token sai | Thiếu Bearer token |
| `403` | Không có quyền | Sửa sách chung không được phép |
| `404` | Không tìm thấy hoặc không thuộc người dùng | ID không hợp lệ về quyền truy cập |
| `409` | Trùng/xung đột dữ liệu | Sách đã có trong thư viện |
| `500` | Lỗi máy chủ | Lỗi không dự kiến |

---

## 5. Danh sách API thống nhất

**Tất cả endpoint dưới đây có tiền tố `/api/v1`.** Trừ đăng ký, đăng nhập và tìm kiếm sách công khai (nếu nhóm chọn công khai), các API phải xác thực.

### 5.1 Tài khoản

| Method | Endpoint | Chức năng | Body / Query chính |
|---|---|---|---|
| POST | `/auth/register` | Đăng ký | `username`, `email`, `password` |
| POST | `/auth/login` | Đăng nhập | `email`, `password` |
| GET | `/users/me` | Hồ sơ hiện tại | Token |
| PATCH | `/users/me` | Đổi tên hiển thị | `username` |

**Ví dụ đăng ký:**

```json
{ "username": "Minh", "email": "minh@example.com", "password": "MatKhauManh123!" }
```

**Ví dụ phản hồi đăng nhập:**

```json
{
  "success": true,
  "data": {
    "access_token": "<JWT_TOKEN>",
    "token_type": "Bearer",
    "user": { "user_id": 1, "username": "Minh", "email": "minh@example.com" }
  }
}
```

### 5.2 Sách và tên phân loại tùy chọn

| Method | Endpoint | Chức năng | Body / Query chính |
|---|---|---|---|
| GET | `/books` | Tìm/lọc sách | `search`, `category_name`, `page`, `limit` |
| GET | `/books/:book_id` | Chi tiết sách | — |
| POST | `/books` | Tạo thông tin sách | `title`, `category_name`, `author`, `total_pages`, `cover_url`, `description`, `publisher`, `published_year`, `isbn` |
| PATCH | `/books/:book_id` | Sửa thông tin sách | Các trường được phép sửa |

**Ví dụ tạo sách:**

```json
{
  "title": "Đắc Nhân Tâm",
  "author": "Dale Carnegie",
  "category_name": "Kỹ năng sống",
  "total_pages": 320,
  "description": "Sách kỹ năng giao tiếp",
  "cover_url": null,
  "publisher": null,
  "published_year": null,
  "isbn": null
}
```

`POST /books` chỉ tạo thông tin sách; **chưa tự thêm vào thư viện**. Để tránh Flutter phải gọi hai API cho thao tác “Thêm sách mới vào thư viện”, có thể dùng endpoint gộp bên dưới.

### 5.3 Thư viện cá nhân

| Method | Endpoint | Chức năng | Body / Query chính |
|---|---|---|---|
| GET | `/me/books` | Danh sách thư viện | `status`, `favorite`, `search`, `page`, `limit` |
| GET | `/me/books/:user_book_id` | Chi tiết sách trong thư viện | — |
| POST | `/me/books` | Thêm sách có sẵn | `book_id` |
| POST | `/me/books/new` | Tạo sách mới và thêm thư viện (transaction) | Các trường của `books` |
| PATCH | `/me/books/:user_book_id` | Đánh dấu yêu thích | `is_favorite` |
| DELETE | `/me/books/:user_book_id` | Bỏ sách khỏi thư viện | Theo chính sách xóa NV-08 |

**Ví dụ thêm sách có sẵn:**

```json
{ "book_id": 10 }
```

**Ví dụ trả về thư viện:**

```json
{
  "success": true,
  "data": [
    {
      "user_book_id": 21,
      "book_id": 10,
      "title": "Đắc Nhân Tâm",
      "author": "Dale Carnegie",
      "cover_url": null,
      "total_pages": 320,
      "status": "READING",
      "is_favorite": true,
      "current_page": 45,
      "active_session_id": 31,
      "progress_percent": 14.06
    }
  ],
  "pagination": { "page": 1, "limit": 20, "total": 1 }
}
```

`current_page`, `active_session_id`, `progress_percent` là **trường API tổng hợp**, không phải cột trực tiếp của `user_books`.

### 5.4 Lượt đọc và tiến độ

| Method | Endpoint | Chức năng | Body |
|---|---|---|---|
| GET | `/me/books/:user_book_id/sessions` | Lịch sử các lượt đọc | — |
| POST | `/me/books/:user_book_id/sessions` | Bắt đầu đọc/đọc lại | `start_date` (tùy chọn) |
| PATCH | `/sessions/:session_id/progress` | Cập nhật trang hiện tại | `current_page` |
| POST | `/sessions/:session_id/complete` | Hoàn thành lượt đọc | `end_date` (tùy chọn) |

**Ví dụ cập nhật tiến độ:**

```json
{ "current_page": 45 }
```

**Ví dụ response:**

```json
{
  "success": true,
  "data": {
    "session_id": 31,
    "user_book_id": 21,
    "session_number": 1,
    "current_page": 45,
    "total_pages": 320,
    "status": "READING",
    "progress_percent": 14.06
  }
}
```

### 5.5 Nhật ký đọc sách

| Method | Endpoint | Chức năng | Body / Query |
|---|---|---|---|
| GET | `/reading-logs` | Lấy nhật ký của tài khoản | `session_id`, `from_date`, `to_date`, `page`, `limit` |
| GET | `/reading-logs/:log_id` | Chi tiết nhật ký | — |
| POST | `/reading-logs` | Thêm nhật ký | `session_id`, `read_date`, `start_page`, `end_page`, `content` |
| PATCH | `/reading-logs/:log_id` | Sửa nhật ký | Các trường được phép sửa |
| DELETE | `/reading-logs/:log_id` | Xóa nhật ký | — |

**Ví dụ:**

```json
{
  "session_id": 31,
  "read_date": "2026-10-11",
  "start_page": 26,
  "end_page": 45,
  "content": "Chương này rất thú vị."
}
```

### 5.6 Ghi chú và đánh giá

| Method | Endpoint | Chức năng | Body |
|---|---|---|---|
| GET | `/me/books/:user_book_id/notes` | Lấy ghi chú | — |
| POST | `/me/books/:user_book_id/notes` | Thêm ghi chú | `page_number`, `content` |
| PATCH | `/notes/:note_id` | Sửa ghi chú | `page_number`, `content` |
| DELETE | `/notes/:note_id` | Xóa ghi chú | — |
| GET | `/me/books/:user_book_id/review` | Lấy đánh giá | — |
| PUT | `/me/books/:user_book_id/review` | Tạo hoặc thay đánh giá | `rating`, `comment` |
| DELETE | `/me/books/:user_book_id/review` | Xóa đánh giá | — |

### 5.7 Mục tiêu đọc sách

| Method | Endpoint | Chức năng | Body / Query |
|---|---|---|---|
| GET | `/goals` | Danh sách mục tiêu | `goal_type`, `page`, `limit` |
| GET | `/goals/:goal_id` | Chi tiết và tiến độ | — |
| POST | `/goals` | Tạo mục tiêu | `goal_name`, `goal_type`, `target_value`, `start_date`, `end_date` |
| PATCH | `/goals/:goal_id` | Sửa mục tiêu | Các trường được phép sửa |
| DELETE | `/goals/:goal_id` | Xóa mục tiêu | — |

**Ví dụ:**

```json
{
  "goal_name": "Đọc 10 cuốn trong học kỳ",
  "goal_type": "BOOKS",
  "target_value": 10,
  "start_date": "2026-10-01",
  "end_date": "2027-01-31"
}
```

**Ví dụ phản hồi tiến độ:**

```json
{
  "success": true,
  "data": {
    "goal_id": 3,
    "goal_name": "Đọc 10 cuốn trong học kỳ",
    "goal_type": "BOOKS",
    "target_value": 10,
    "current_value": 4,
    "progress_percent": 40,
    "is_completed": false
  }
}
```

`current_value`, `progress_percent`, `is_completed` là trường Backend tính khi trả kết quả.

### 5.8 Trang chủ và thống kê

| Method | Endpoint | Chức năng |
|---|---|---|
| GET | `/dashboard` | Dữ liệu tổng quan trang chủ |
| GET | `/statistics?from_date=YYYY-MM-DD&to_date=YYYY-MM-DD` | Thống kê theo thời gian |

**Ví dụ response thống kê:**

```json
{
  "success": true,
  "data": {
    "total_books": 15,
    "want_to_read_books": 3,
    "reading_books": 4,
    "completed_books": 8,
    "total_pages_read": 1200,
    "total_reading_logs": 25,
    "completed_goals": 2
  }
}
```

Số liệu trên là **ví dụ minh họa**, không phải dữ liệu thực tế. Cần chốt cách đếm trang đọc và hoàn thành mục tiêu trước khi lập trình.

---

## 6. Ma trận Frontend ↔ Backend ↔ Database

| Màn hình / hành động Flutter | API | Bảng đọc/ghi |
|---|---|---|
| Đăng ký | `POST /auth/register` | `users` (ghi) |
| Đăng nhập | `POST /auth/login` | `users` (đọc) |
| Hồ sơ / đổi tên | `GET/PATCH /users/me` | `users` |
| Trang chủ | `GET /dashboard` | `user_books`, `reading_sessions`, `reading_goals`, `reading_logs` |
| Tìm/lọc sách theo tên phân loại | `GET /books` | `books` |
| Thêm sách mới | `POST /me/books/new` | `books`, `user_books` (ghi) |
| Thêm sách có sẵn | `POST /me/books` | `user_books` (ghi) |
| Thư viện / lọc / yêu thích | `GET/PATCH /me/books` | `user_books`, `books` |
| Bắt đầu đọc | `POST /me/books/:id/sessions` | `reading_sessions`, `user_books` |
| Cập nhật trang | `PATCH /sessions/:id/progress` | `reading_sessions` |
| Hoàn thành / đọc lại | `POST /sessions/:id/complete` và tạo session mới | `reading_sessions`, `user_books` |
| Nhật ký | `/reading-logs` | `reading_logs` |
| Ghi chú | `/notes`, `/me/books/:id/notes` | `book_notes` |
| Đánh giá | `/me/books/:id/review` | `reviews` |
| Mục tiêu | `/goals` | `reading_goals` và dữ liệu đọc |
| Thống kê | `GET /statistics` | Truy vấn tổng hợp, không tạo bảng riêng |
| Đổi theme sáng/tối | Không cần API | `SharedPreferences` trên Flutter |

**Gợi ý tổ chức Flutter:** `ApiClient` (gửi HTTP, token, xử lý lỗi) → `AuthService`, `BookService`, `ReadingService`, `GoalService`, `StatisticsService` → `JournalStore`/state management → UI. Dữ liệu nghiệp vụ chính không còn phụ thuộc vào `SharedPreferences`.

---

## 7. Quy ước khi lập trình để tránh lệch Frontend/Backend

1. **Tên trường:** dùng `snake_case` trong JSON (`total_pages`, không dùng lẫn `totalPages`). Flutter có thể map sang model Dart theo quy ước riêng.
2. **Kiểu dữ liệu:** ID và trang là số nguyên; `is_favorite` là boolean; `rating` là số nguyên 1–5; ngày dùng `YYYY-MM-DD`.
3. **Giá trị trạng thái:** chỉ dùng `WANT_TO_READ`, `READING`, `COMPLETED`; mục tiêu chỉ `BOOKS`, `PAGES`.
4. **Lỗi:** UI hiển thị `error.message`, không phụ thuộc vào nội dung lỗi PostgreSQL.
5. **Đăng nhập:** mọi API cá nhân đều gắn Bearer token; nếu nhận `401`, Flutter chuyển sang luồng đăng nhập lại.
6. **Pagination:** mặc định `page=1`, `limit=20`; Backend giới hạn `limit` tối đa (đề xuất 100).
7. **Không gửi:** `password_hash`, SQL lỗi nội bộ, hoặc dữ liệu cá nhân của người khác.
8. **Không để UI tự tính nghiệp vụ quan trọng:** trạng thái đọc, tiến độ mục tiêu và thống kê lấy từ Backend.
9. **Kiểm thử API trước khi nối Flutter:** dùng Postman/Thunder Client và kiểm tra dữ liệu trong pgAdmin.
10. **Bảo mật:** dùng truy vấn tham số hóa (`$1`, `$2`...), băm mật khẩu, kiểm tra quyền sở hữu từng tài nguyên; không đưa `.env` lên Git.

---

## 8. Những quyết định nhóm cần chốt trước khi code

| Vấn đề | Phương án đề xuất | Lý do |
|---|---|---|
| Sách tự thêm có công khai không? | Mặc định chỉ chủ sở hữu nhìn thấy; sách chung do hệ thống quản lý | Tránh lộ sách riêng |
| Xóa sách có lịch sử đọc? | Tạm chặn (`409`) và xác nhận yêu cầu | Tránh mất nhật ký ngoài ý muốn |
| Viết nhật ký có cập nhật tiến độ? | Không tự cập nhật | Tránh hai nguồn thay đổi tiến độ |
| Đọc lại tính vào mục tiêu BOOKS? | Không, chỉ đếm sách duy nhất | Tránh tăng số sách ảo |
| Trang đã đọc được tính thế nào? | Cần chọn: cộng trang mỗi nhật ký hay đếm trang duy nhất | Tránh trùng khoảng trang |
| Sách không có ảnh bìa? | `cover_url=null`, Flutter hiển thị ảnh mặc định | Không bắt buộc upload |
| Có upload ảnh bìa từ thiết bị không? | Giai đoạn đầu dùng URL; upload để giai đoạn sau | Cần thêm xử lý file/storage |
| Có màn hình ghi chú riêng? | Nếu chưa có, hoãn API `book_notes` | Tập trung chức năng cốt lõi |
| Có màn hình đánh giá riêng? | Nếu chưa có, hoãn API `reviews` | Tập trung chức năng cốt lõi |

**Cảnh báo kỹ thuật:** Với schema SQL hiện tại, không nên dùng `DELETE` dây chuyền mà chưa thiết kế chính sách dữ liệu. Nếu muốn hỗ trợ xóa sách cùng nhật ký, cần thống nhất trước rồi sửa FK hoặc xử lý xóa có kiểm soát bằng transaction.

---

## 9. Kế hoạch triển khai và bàn giao giữa hai thành viên

| Giai đoạn | Backend | Frontend | Tiêu chí hoàn thành |
|---|---|---|---|
| 1. Nền tảng | Go, Echo, GORM, `.env`, kết nối PostgreSQL, chuẩn lỗi | `ApiClient`, base URL, model dữ liệu | Flutter gọi được `/health` |
| 2. Tài khoản | Register, login, JWT, `/users/me` | Form đăng nhập/đăng ký, lưu token an toàn | Hai tài khoản không xem được dữ liệu của nhau |
| 3. Sách | books (`category_name` dạng chuỗi), user_books | Thêm sách, danh sách, chi tiết, yêu thích | Thêm sách → reload vẫn còn |
| 4. Đọc sách | Sessions, progress, complete, reread | Nút bắt đầu, tiến độ, hoàn thành, đọc lại | Đọc lại không mất lịch sử |
| 5. Nhật ký | CRUD logs, kiểm tra trang | Form và danh sách nhật ký | Tạo/sửa/xóa đồng bộ PostgreSQL |
| 6. Mục tiêu | CRUD goals, tính tiến độ | Tạo mục tiêu, thanh tiến độ | Tiến độ trả về đúng |
| 7. Thống kê | Dashboard, statistics | Thẻ thống kê, biểu đồ | Số liệu khớp dữ liệu đọc |
| 8. Mở rộng | Notes, reviews, upload ảnh (nếu cần) | Màn hình tương ứng | Không làm hỏng tính năng cũ |
| 9. Kiểm thử | Test API, phân quyền, lỗi | Test luồng UI, xử lý mất mạng | Không crash, dữ liệu nhất quán |

### Checklist nghiệm thu tối thiểu

- [ ] Đăng ký, đăng nhập, gọi API có token.
- [ ] Thêm sách mới và thêm sách có sẵn vào thư viện.
- [ ] Không thêm trùng sách trong cùng thư viện.
- [ ] Bắt đầu đọc, cập nhật trang, hoàn thành.
- [ ] Đọc lại sách vẫn xem được nhật ký lượt cũ.
- [ ] Thêm, sửa, xóa nhật ký đúng quyền sở hữu.
- [ ] Tạo, sửa, xóa mục tiêu.
- [ ] Thống kê đúng theo quy tắc nhóm đã chốt.
- [ ] Dữ liệu vẫn còn sau khi tắt/mở lại app.
- [ ] Hai tài khoản không truy cập được dữ liệu riêng của nhau.
- [ ] Không còn dùng dữ liệu giả hoặc `SharedPreferences` làm nguồn dữ liệu nghiệp vụ chính.

---

## 10. Phân biệt đã thống nhất và còn đề xuất

**Đã thống nhất từ trao đổi:** Flutter + Go/Echo/GORM + PostgreSQL; 8 bảng nghiệp vụ; `category_name` là chuỗi tùy chọn trên sách, không quản lý category riêng; không có ảnh đại diện; sách có `cover_url`; nghiệp vụ quản lý sách, nhật ký, mục tiêu, thống kê và đọc lại.

**Cần xác nhận trước khi triển khai chính thức:** API route chi tiết, quy tắc quyền riêng tư sách tự tạo, xóa dữ liệu có lịch sử, cách tính trang đọc, phạm vi ghi chú/đánh giá và upload ảnh bìa. Những phần này được đề xuất để Frontend/Backend có một hợp đồng làm việc rõ ràng.

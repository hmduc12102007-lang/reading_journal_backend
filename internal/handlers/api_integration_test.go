package handlers_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reading_journal_backend/internal/auth"
	"reading_journal_backend/internal/config"
	"reading_journal_backend/internal/database"
	"reading_journal_backend/internal/handlers"
	"reading_journal_backend/internal/models"
	"reading_journal_backend/internal/routes"

	"github.com/labstack/echo/v4"
)

// All DDL and fixtures are confined to a fresh, randomly named schema.
// This test never migrates or modifies the application's existing tables.
func TestAPIIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	db, err := database.Connect(config.Config{DBConnectionString: dsn})
	if err != nil {
		t.Fatal("cannot connect to PostgreSQL test server (check TEST_DATABASE_URL)")
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "codex_test_" + hex.EncodeToString(random[:])
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	adminDB := db
	defer func() {
		if err := adminDB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
	}()
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err = database.Connect(config.Config{DBConnectionString: u.String()})
	if err != nil {
		t.Fatal("cannot connect to isolated test schema")
	}
	testConnection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer testConnection.Close()
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("repeated migration: %v", err)
	}
	storage := t.TempDir()
	secret := strings.Repeat("test-secret-", 4)
	e := echo.New()
	routes.Register(e, routes.Dependencies{
		AuthHandler: handlers.NewAuthHandler(db, secret), BookHandler: handlers.NewBookHandler(db, storage),
		ReadingHandler: handlers.NewReadingHandler(db), NotesReviewsHandler: handlers.NewNotesReviewsHandler(db),
		GoalHandler: handlers.NewGoalHandler(db), AuthMiddleware: auth.Middleware(db, secret), OptionalAuth: auth.OptionalMiddleware(db, secret),
	})
	call := func(method, path, token, body string, status int) map[string]json.RawMessage {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, rec.Code, status, rec.Body.String())
		}
		var response map[string]json.RawMessage
		if rec.Body.Len() != 0 {
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}
		return response
	}
	register := func(email string) string {
		t.Helper()
		res := call("POST", "/auth/register", "", fmt.Sprintf(`{"username":"Reader","email":%q,"password":"Password123!"}`, email), 201)
		var data struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(res["data"], &data); err != nil {
			t.Fatal(err)
		}
		return data.AccessToken
	}
	owner, other := register("owner@example.com"), register("other@example.com")
	call("POST", "/auth/register", "", `{"username":"Reader","email":"owner@example.com","password":"Password123!"}`, 409)
	call("POST", "/auth/login", "", `{"email":"owner@example.com","password":"wrong"}`, 401)
	response := call("POST", "/me/books/new", owner, `{"title":"Test Book","author":"Author","total_pages":100}`, 201)
	var library handlers.LibraryBook
	if err := json.Unmarshal(response["data"], &library); err != nil {
		t.Fatal(err)
	}
	bookPath := fmt.Sprintf("/books/%d", library.BookID)
	coverPath := bookPath + "/cover"
	call("GET", bookPath, other, "", 404)
	call("PATCH", bookPath, owner, `{"author":" "}`, 400)
	call("PATCH", bookPath, owner, `{"cover_url":"file:///secret"}`, 400)
	call("GET", "/books?page=9223372036854775807&limit=100", owner, "", 400)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	upload := func(token string, data []byte, status int) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "../../untrusted.exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/v1"+coverPath, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("upload: got %d, want %d: %s", rec.Code, status, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "cover_file") {
			t.Fatal("internal storage path leaked")
		}
	}
	upload("", imageBytes.Bytes(), 401)
	upload(other, imageBytes.Bytes(), 404)
	upload(owner, []byte("fake image"), 400)
	upload(owner, make([]byte, (5<<20)+1), 413)
	upload(owner, imageBytes.Bytes(), 200)
	var book models.Book
	if err := db.First(&book, "book_id = ?", library.BookID).Error; err != nil {
		t.Fatal(err)
	}
	if book.CoverFile == nil || book.CoverURL == nil || *book.CoverURL != "/api/v1"+coverPath {
		t.Fatal("missing stored cover metadata")
	}
	oldFile := filepath.Join(storage, *book.CoverFile)
	for _, token := range []string{"", other, owner} {
		req := httptest.NewRequest("GET", "/api/v1"+coverPath, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if token == owner {
			if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), imageBytes.Bytes()) || rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("failed to retrieve private cover")
			}
		} else if rec.Code != 404 {
			t.Fatal("private cover visible to another user")
		}
	}
	upload(owner, imageBytes.Bytes(), 200)
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old cover file was not removed")
	}
	call("PATCH", bookPath, owner, fmt.Sprintf(`{"cover_url":%q}`, "/api/v1"+coverPath), 200)
	sameCoverFiles, err := os.ReadDir(storage)
	if err != nil || len(sameCoverFiles) != 1 {
		t.Fatal("unchanged cover URL detached the uploaded file")
	}
	call("PATCH", bookPath, owner, `{"cover_url":"https://example.com/cover.png"}`, 200)
	externalCoverFiles, err := os.ReadDir(storage)
	if err != nil || len(externalCoverFiles) != 0 {
		t.Fatal("external cover replacement left local files")
	}
	upload(owner, imageBytes.Bytes(), 200)
	call("PATCH", bookPath, owner, `{"cover_url":null}`, 200)
	files, err := os.ReadDir(storage)
	if err != nil || len(files) != 0 {
		t.Fatal("clearing cover left unused files")
	}
	upload(owner, imageBytes.Bytes(), 200)
	call("DELETE", coverPath, other, "", 404)
	call("DELETE", coverPath, owner, "", 204)
	call("GET", coverPath, owner, "", 404)
	files, err = os.ReadDir(storage)
	if err != nil || len(files) != 0 {
		t.Fatal("deleting cover left unused files")
	}
	libraryPath := fmt.Sprintf("/me/books/%d", library.UserBookID)
	res := call("POST", libraryPath+"/sessions", owner, `{"start_date":"2026-10-01"}`, 201)
	var session handlers.SessionResponse
	if err := json.Unmarshal(res["data"], &session); err != nil {
		t.Fatal(err)
	}
	sessionPath := fmt.Sprintf("/sessions/%d", session.SessionID)
	call("POST", libraryPath+"/sessions", owner, `{}`, 200)
	call("PATCH", sessionPath+"/progress", other, `{"current_page":10}`, 404)
	call("PATCH", sessionPath+"/progress", owner, `{"current_page":101}`, 400)
	call("PATCH", sessionPath+"/progress", owner, `{"current_page":30}`, 200)
	logBody := fmt.Sprintf(`{"session_id":%d,"read_date":"2026-10-02","start_page":1,"end_page":30,"content":"Notes"}`, session.SessionID)
	logRes := call("POST", "/reading-logs", owner, logBody, 201)
	var log models.ReadingLog
	if err := json.Unmarshal(logRes["data"], &log); err != nil {
		t.Fatal(err)
	}
	call("PATCH", bookPath, owner, `{"total_pages":20}`, 409)
	call("POST", libraryPath+"/notes", owner, `{"page_number":101,"content":"Note"}`, 400)
	call("POST", libraryPath+"/notes", owner, `{"page_number":20,"content":"Note"}`, 201)
	call("PUT", libraryPath+"/review", owner, `{"rating":5,"comment":"Great"}`, 200)
	call("PUT", libraryPath+"/review", owner, `{"rating":4}`, 200)
	call("POST", "/goals", owner, `{"goal_name":"Read","goal_type":"PAGES","target_value":30,"start_date":"2026-10-01","end_date":"2026-10-31"}`, 201)
	call("POST", "/goals", owner, `{"goal_name":" ","goal_type":"BOOKS","target_value":1,"start_date":"2026-10-01","end_date":"2026-10-31"}`, 400)
	call("POST", sessionPath+"/complete", owner, `{"end_date":"2026-09-30"}`, 400)
	completed := call("POST", sessionPath+"/complete", owner, `{"end_date":"2026-10-03"}`, 200)
	if err := json.Unmarshal(completed["data"], &session); err != nil {
		t.Fatal(err)
	}
	if session.CurrentPage != 100 || session.Progress != 100 {
		t.Fatal("completed session did not reach 100 percent")
	}
	completedLibrary := call("GET", libraryPath, owner, "", 200)
	if err := json.Unmarshal(completedLibrary["data"], &library); err != nil {
		t.Fatal(err)
	}
	if library.CurrentPage != 100 || library.ProgressPercent != 100 || library.ActiveSessionID != nil {
		t.Fatal("completed library book progress is incorrect")
	}
	call("POST", sessionPath+"/complete", owner, `{}`, 409)
	call("GET", "/dashboard", owner, "", 200)
	call("GET", "/statistics", owner, "", 200)
	call("GET", "/goals", owner, "", 200)
	call("GET", "/me/books/not-a-number/notes", owner, "", 400)
	reread := call("POST", libraryPath+"/sessions", owner, `{"start_date":"2026-10-04"}`, 201)
	if err := json.Unmarshal(reread["data"], &session); err != nil {
		t.Fatal(err)
	}
	if session.SessionNumber != 2 || session.CurrentPage != 0 {
		t.Fatal("rereading did not start a fresh session")
	}
	call("DELETE", libraryPath, owner, "", 409)
	call("PATCH", "/auth/change-password", owner, `{"current_password":"Password123!","new_password":"Different123!"}`, 200)
	call("GET", "/users/me", owner, "", 401)
}

package handlers

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"reading_journal_backend/internal/models"
)

func TestCoverValidation(t *testing.T) {
	var pngBytes, jpegBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBytes, img, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		ext  string
	}{
		{"PNG", pngBytes.Bytes(), ".png"}, {"JPEG", jpegBytes.Bytes(), ".jpg"},
		{"fake image", []byte("<script>alert(1)</script>"), ""},
		{"truncated PNG", pngBytes.Bytes()[:40], ""},
		{"empty", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ext, err := coverExtension(tc.data)
			if tc.ext == "" {
				if err == nil {
					t.Fatal("accepted invalid image")
				}
				return
			}
			if err != nil || ext != tc.ext {
				t.Fatalf("extension = %q, error = %v", ext, err)
			}
		})
	}
	// A tiny compressed payload must not trigger an enormous decoded allocation.
	huge := append([]byte(nil), pngBytes.Bytes()...)
	binary.BigEndian.PutUint32(huge[16:20], 100_000)
	binary.BigEndian.PutUint32(huge[20:24], 100_000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	if _, err := coverExtension(huge); err == nil || !strings.Contains(err.Error(), "pixels") {
		t.Fatalf("oversized image: %v", err)
	}
}

func TestSafeCoverName(t *testing.T) {
	for _, name := range []string{"../secret.png", `..\secret.png`, "C:/secret.png", "file.svg", strings.Repeat("a", 32) + ".jpg.exe"} {
		if safeCoverName(&name) {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	name := strings.Repeat("a", 32) + ".png"
	if !safeCoverName(&name) || safeCoverName(nil) {
		t.Fatal("invalid safe-name validation")
	}
}

func TestBookValidation(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "file:///tmp/x", "/uploads/a.png", "https://user:password@example.com/a", "https://"} {
		_, message := makeBook(BookRequest{Title: "Book", Author: "Author", TotalPages: 10, CoverURL: &value}, nil)
		if message == "" {
			t.Fatalf("accepted invalid URL %q", value)
		}
	}
	for _, req := range []BookRequest{
		{Title: strings.Repeat("a", 256), Author: "Author", TotalPages: 10},
		{Title: "Book", Author: " ", TotalPages: 10},
	} {
		if _, message := makeBook(req, nil); message == "" {
			t.Fatal("accepted invalid book")
		}
	}
	goal := models.ReadingGoal{GoalName: "   ", GoalType: "BOOKS", TargetValue: 1}
	if validateGoal(goal) == "" {
		t.Fatal("accepted whitespace goal name")
	}
}

func TestPaginationOverflow(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest("GET", "/?page="+strconv.Itoa(int(^uint(0)>>1))+"&limit=100", nil)
	if _, _, err := parsePagination(e.NewContext(request, httptest.NewRecorder())); err == nil {
		t.Fatal("accepted overflowing offset")
	}
}

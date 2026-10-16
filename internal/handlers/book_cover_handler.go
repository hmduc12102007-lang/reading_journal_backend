package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reading_journal_backend/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxCoverBytes = 5 << 20
const maxCoverPixels = 20_000_000

func coverURL(bookID uint64) string { return fmt.Sprintf("/api/v1/books/%d/cover", bookID) }

// Inspect actual image bytes, independent of the client filename and MIME header.
func coverExtension(data []byte) (string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("file must be a valid JPEG, PNG, or GIF image")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxCoverPixels/config.Height {
		return "", errors.New("image must contain at most 20 million pixels")
	}
	extensions := map[string]string{"jpeg": ".jpg", "png": ".png", "gif": ".gif"}
	ext, ok := extensions[format]
	if !ok {
		return "", errors.New("only JPEG, PNG, and GIF images are supported")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", errors.New("image is corrupt or incomplete")
	}
	return ext, nil
}

func (h *BookHandler) UploadCover(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, 401, "UNAUTHORIZED", "Authentication required", nil)
	}
	bookID, err := parseID(c.Param("book_id"))
	if err != nil {
		return apiError(c, 400, "VALIDATION_ERROR", "book_id must be a positive integer", nil)
	}
	var book models.Book
	if err := h.visibleBooks(c).First(&book, "book_id = ?", bookID).Error; err != nil {
		return coverLookupError(c, err)
	}
	if book.CreatedBy == nil || *book.CreatedBy != userID {
		return apiError(c, 403, "FORBIDDEN", "Only the creator can change this cover", nil)
	}
	// Enforce the request limit even when this handler is used without global middleware.
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxCoverBytes+(1<<20))
	err = c.Request().ParseMultipartForm(1 << 20)
	if c.Request().MultipartForm != nil {
		defer c.Request().MultipartForm.RemoveAll()
	}
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			return apiError(c, 413, "PAYLOAD_TOO_LARGE", "Image must be at most 5 MiB", nil)
		}
		return apiError(c, 400, "VALIDATION_ERROR", "Send multipart/form-data with one file field named image", nil)
	}
	files := c.Request().MultipartForm.File["image"]
	if len(files) != 1 || len(c.Request().MultipartForm.File) != 1 {
		return apiError(c, 400, "VALIDATION_ERROR", "Exactly one image file is required", nil)
	}
	if files[0].Size > maxCoverBytes {
		return apiError(c, 413, "PAYLOAD_TOO_LARGE", "Image must be at most 5 MiB", nil)
	}
	file, err := files[0].Open()
	if err != nil {
		return apiError(c, 400, "VALIDATION_ERROR", "Cannot read uploaded image", nil)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCoverBytes+1))
	if err != nil {
		return apiError(c, 400, "VALIDATION_ERROR", "Cannot read uploaded image", nil)
	}
	if len(data) > maxCoverBytes {
		return apiError(c, 413, "PAYLOAD_TOO_LARGE", "Image must be at most 5 MiB", nil)
	}
	ext, err := coverExtension(data)
	if err != nil {
		return apiError(c, 400, "VALIDATION_ERROR", err.Error(), nil)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return apiError(c, 500, "INTERNAL_ERROR", "Failed to generate image name", nil)
	}
	name := hex.EncodeToString(random[:]) + ext
	destination, err := os.OpenFile(filepath.Join(h.uploadDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return apiError(c, 500, "INTERNAL_ERROR", "Failed to store image", nil)
	}
	_, writeErr := destination.Write(data)
	closeErr := destination.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(filepath.Join(h.uploadDir, name))
		return apiError(c, 500, "INTERNAL_ERROR", "Failed to store image", nil)
	}
	var previous *string
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&book, "book_id = ? AND created_by = ?", bookID, userID).Error; err != nil {
			return err
		}
		previous = copyCoverName(book.CoverFile)
		url := coverURL(bookID)
		if err := tx.Model(&book).Updates(map[string]interface{}{"cover_url": url, "cover_file": name}).Error; err != nil {
			return err
		}
		book.CoverURL, book.CoverFile = &url, &name
		return nil
	})
	if err != nil {
		_ = os.Remove(filepath.Join(h.uploadDir, name))
		return coverLookupError(c, err)
	}
	h.removeUnusedCover(c, previous)
	return apiSuccess(c, http.StatusOK, book)
}

func (h *BookHandler) GetCover(c echo.Context) error {
	bookID, err := parseID(c.Param("book_id"))
	if err != nil {
		return apiError(c, 400, "VALIDATION_ERROR", "book_id must be a positive integer", nil)
	}
	var book models.Book
	if err := h.visibleBooks(c).First(&book, "book_id = ?", bookID).Error; err != nil {
		return coverLookupError(c, err)
	}
	if !safeCoverName(book.CoverFile) {
		return apiError(c, 404, "NOT_FOUND", "Uploaded cover not found", nil)
	}
	file, err := os.Open(filepath.Join(h.uploadDir, *book.CoverFile))
	if errors.Is(err, os.ErrNotExist) {
		return apiError(c, 404, "NOT_FOUND", "Uploaded cover not found", nil)
	}
	if err != nil {
		return apiError(c, 500, "INTERNAL_ERROR", "Failed to read cover", nil)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return apiError(c, 500, "INTERNAL_ERROR", "Failed to read cover", nil)
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Response(), c.Request(), info.Name(), info.ModTime(), file)
	return nil
}

func (h *BookHandler) DeleteCover(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, 401, "UNAUTHORIZED", "Authentication required", nil)
	}
	bookID, err := parseID(c.Param("book_id"))
	if err != nil {
		return apiError(c, 400, "VALIDATION_ERROR", "book_id must be a positive integer", nil)
	}
	var previous *string
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var book models.Book
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&book, "book_id = ? AND created_by = ?", bookID, userID).Error; err != nil {
			return err
		}
		previous = copyCoverName(book.CoverFile)
		return tx.Model(&book).Updates(map[string]interface{}{"cover_url": nil, "cover_file": nil}).Error
	})
	if err != nil {
		return coverLookupError(c, err)
	}
	h.removeUnusedCover(c, previous)
	return c.NoContent(http.StatusNoContent)
}

func safeCoverName(name *string) bool {
	if name == nil {
		return false
	}
	ext := filepath.Ext(*name)
	if ext != ".jpg" && ext != ".png" && ext != ".gif" {
		return false
	}
	stem := strings.TrimSuffix(*name, ext)
	decoded, err := hex.DecodeString(stem)
	return err == nil && len(decoded) == 16 && len(stem) == 32
}

func copyCoverName(name *string) *string {
	if name == nil {
		return nil
	}
	value := *name
	return &value
}

func (h *BookHandler) removeUnusedCover(c echo.Context, name *string) {
	if !safeCoverName(name) {
		return
	}
	var count int64
	if err := h.db.Model(&models.Book{}).Where("cover_file = ?", *name).Count(&count).Error; err != nil {
		c.Logger().Errorf("check unused cover: %v", err)
		return
	}
	if count != 0 {
		return
	}
	if err := os.Remove(filepath.Join(h.uploadDir, *name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		c.Logger().Errorf("remove unused cover: %v", err)
	}
}

func coverLookupError(c echo.Context, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, 404, "NOT_FOUND", "Book not found", nil)
	}
	return apiError(c, 500, "INTERNAL_ERROR", "Failed to access book cover", nil)
}

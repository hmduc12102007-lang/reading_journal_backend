package handlers

import (
	"errors"
	"net/http"
	"strings"

	"reading_journal_backend/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotesReviewsHandler struct {
	db *gorm.DB
}

func NewNotesReviewsHandler(db *gorm.DB) *NotesReviewsHandler {
	return &NotesReviewsHandler{db: db}
}

func (h *NotesReviewsHandler) ListNotes(c echo.Context) error {
	userID, userBookID, ok := h.ownedUserBook(c)
	if !ok {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	notes := make([]models.BookNote, 0)
	if err := h.db.Where("user_book_id = ?", userBookID).Order("created_at DESC").Find(&notes).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list notes", nil)
	}
	_ = userID
	return apiSuccess(c, http.StatusOK, notes)
}

func (h *NotesReviewsHandler) CreateNote(c echo.Context) error {
	userID, userBookID, ok := h.ownedUserBook(c)
	if !ok {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	var req struct {
		PageNumber *int   `json:"page_number"`
		Content    string `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	if strings.TrimSpace(req.Content) == "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "content is required", nil)
	}
	var userBook models.UserBook
	var book models.Book
	if err := h.db.First(&userBook, "user_book_id = ? AND user_id = ?", userBookID, userID).Error; err != nil {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	if err := h.db.First(&book, "book_id = ?", userBook.BookID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load book", nil)
	}
	if req.PageNumber != nil && (*req.PageNumber < 1 || *req.PageNumber > book.TotalPages) {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "page_number must be within the book page range", nil)
	}
	note := models.BookNote{UserBookID: userBookID, PageNumber: req.PageNumber, Content: strings.TrimSpace(req.Content)}
	if err := h.db.Create(&note).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create note", nil)
	}
	return apiSuccess(c, http.StatusCreated, note)
}

func (h *NotesReviewsHandler) UpdateNote(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	noteID, err := parseID(c.Param("note_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "note_id must be a positive integer", nil)
	}
	var note models.BookNote
	if err := h.db.Table("book_notes AS n").Select("n.*").Joins("JOIN user_books AS ub ON ub.user_book_id = n.user_book_id").Where("n.note_id = ? AND ub.user_id = ?", noteID, userID).Scan(&note).Error; err != nil || note.NoteID == 0 {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Note not found", nil)
	}
	var req struct {
		PageNumber OptionalField[int] `json:"page_number"`
		Content    *string            `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	if !req.PageNumber.Set && req.Content == nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "At least one field is required", nil)
	}
	var userBook models.UserBook
	if err := h.db.First(&userBook, "user_book_id = ?", note.UserBookID).Error; err != nil {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Note not found", nil)
	}
	var book models.Book
	if err := h.db.First(&book, "book_id = ?", userBook.BookID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load book", nil)
	}
	updates := map[string]interface{}{}
	if req.PageNumber.Set {
		if req.PageNumber.Null {
			updates["page_number"] = nil
		} else {
			if req.PageNumber.Value < 1 || req.PageNumber.Value > book.TotalPages {
				return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "page_number must be within the book page range or null", nil)
			}
			updates["page_number"] = req.PageNumber.Value
		}
	}
	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "content cannot be empty", nil)
		}
		updates["content"] = strings.TrimSpace(*req.Content)
	}
	if err := h.db.Model(&note).Updates(updates).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update note", nil)
	}
	if err := h.db.First(&note, "note_id = ?", noteID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reload note", nil)
	}
	return apiSuccess(c, http.StatusOK, note)
}

func (h *NotesReviewsHandler) DeleteNote(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	noteID, err := parseID(c.Param("note_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "note_id must be a positive integer", nil)
	}
	result := h.db.Where("note_id = ? AND user_book_id IN (?)", noteID,
		h.db.Model(&models.UserBook{}).Select("user_book_id").Where("user_id = ?", userID)).Delete(&models.BookNote{})
	if result.Error != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete note", nil)
	}
	if result.RowsAffected == 0 {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Note not found", nil)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *NotesReviewsHandler) GetReview(c echo.Context) error {
	userID, userBookID, ok := h.ownedUserBook(c)
	if !ok {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	var review models.Review
	err := h.db.Where("user_book_id = ?", userBookID).First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiSuccess(c, http.StatusOK, nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load review", nil)
	}
	_ = userID
	return apiSuccess(c, http.StatusOK, review)
}

func (h *NotesReviewsHandler) UpsertReview(c echo.Context) error {
	userID, userBookID, ok := h.ownedUserBook(c)
	if !ok {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	var req struct {
		Rating  int     `json:"rating"`
		Comment *string `json:"comment"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	if req.Rating < 1 || req.Rating > 5 {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "rating must be between 1 and 5", nil)
	}
	review := models.Review{UserBookID: userBookID, Rating: req.Rating, Comment: emptyToNil(req.Comment)}
	err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"rating", "comment", "updated_at"}),
	}).Create(&review).Error
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save review", nil)
	}
	if err := h.db.First(&review, "user_book_id = ?", userBookID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load review", nil)
	}
	_ = userID
	return apiSuccess(c, http.StatusOK, review)
}

func (h *NotesReviewsHandler) DeleteReview(c echo.Context) error {
	userID, userBookID, ok := h.ownedUserBook(c)
	if !ok {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	result := h.db.Where("user_book_id = ?", userBookID).Delete(&models.Review{})
	if result.Error != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete review", nil)
	}
	if result.RowsAffected == 0 {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Review not found", nil)
	}
	_ = userID
	return c.NoContent(http.StatusNoContent)
}

func (h *NotesReviewsHandler) ownedUserBook(c echo.Context) (uint64, uint64, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return 0, 0, false
	}
	userBookID, err := parseID(c.Param("user_book_id"))
	if err != nil {
		return userID, 0, false
	}
	var count int64
	err = h.db.Model(&models.UserBook{}).Where("user_book_id = ? AND user_id = ?", userBookID, userID).Count(&count).Error
	return userID, userBookID, err == nil && count == 1
}

package handlers

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"reading_journal_backend/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	statusWantToRead = "WANT_TO_READ"
	statusReading    = "READING"
	statusCompleted  = "COMPLETED"
)

type BookHandler struct {
	db        *gorm.DB
	uploadDir string
}

type BookRequest struct {
	CategoryName  *string `json:"category_name"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	Description   string  `json:"description"`
	TotalPages    int     `json:"total_pages"`
	CoverURL      *string `json:"cover_url"`
	Publisher     *string `json:"publisher"`
	PublishedYear *int    `json:"published_year"`
	ISBN          *string `json:"isbn"`
}

type BookPatchRequest struct {
	CategoryName  OptionalField[string] `json:"category_name"`
	Title         *string               `json:"title"`
	Author        *string               `json:"author"`
	Description   *string               `json:"description"`
	TotalPages    *int                  `json:"total_pages"`
	CoverURL      OptionalField[string] `json:"cover_url"`
	Publisher     OptionalField[string] `json:"publisher"`
	PublishedYear OptionalField[int]    `json:"published_year"`
	ISBN          OptionalField[string] `json:"isbn"`
}

type LibraryBook struct {
	UserBookID      uint64  `json:"user_book_id"`
	BookID          uint64  `json:"book_id"`
	Title           string  `json:"title"`
	Author          string  `json:"author"`
	CategoryName    *string `json:"category_name"`
	CoverURL        *string `json:"cover_url"`
	TotalPages      int     `json:"total_pages"`
	Status          string  `json:"status"`
	IsFavorite      bool    `json:"is_favorite"`
	CurrentPage     int     `json:"current_page"`
	ActiveSessionID *uint64 `json:"active_session_id"`
	ProgressPercent float64 `json:"progress_percent"`
}

func NewBookHandler(db *gorm.DB, uploadDir string) *BookHandler {
	return &BookHandler{db: db, uploadDir: uploadDir}
}

func (h *BookHandler) List(c echo.Context) error {
	page, limit, err := parsePagination(c)
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
	}
	query := h.visibleBooks(c).Model(&models.Book{})
	if search := strings.TrimSpace(c.QueryParam("search")); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("(title ILIKE ? OR author ILIKE ? OR isbn ILIKE ?)", pattern, pattern, pattern)
	}
	if categoryName := strings.TrimSpace(c.QueryParam("category_name")); categoryName != "" {
		query = query.Where("LOWER(category_name) = LOWER(?)", categoryName)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count books", nil)
	}
	books := make([]models.Book, 0)
	if err := query.Order("title ASC").Offset((page - 1) * limit).Limit(limit).Find(&books).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list books", nil)
	}
	return apiListSuccess(c, books, page, limit, total)
}

func (h *BookHandler) Get(c echo.Context) error {
	bookID, err := parseID(c.Param("book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "book_id must be a positive integer", nil)
	}
	var book models.Book
	if err := h.visibleBooks(c).Where("book_id = ?", bookID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apiError(c, http.StatusNotFound, "NOT_FOUND", "Book not found", nil)
		}
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load book", nil)
	}
	return apiSuccess(c, http.StatusOK, book)
}

func (h *BookHandler) Create(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req BookRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	book, message := makeBook(req, &userID)
	if message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	if err := h.db.Create(&book).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create book", nil)
	}
	return apiSuccess(c, http.StatusCreated, book)
}

func (h *BookHandler) Update(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	bookID, err := parseID(c.Param("book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "book_id must be a positive integer", nil)
	}
	var book models.Book
	if err := h.db.First(&book, "book_id = ?", bookID).Error; err != nil {
		return lookupError(c, err, "Book not found", "Failed to load book")
	}
	if book.CreatedBy == nil || *book.CreatedBy != userID {
		return apiError(c, http.StatusForbidden, "FORBIDDEN", "Only the creator can update this book", nil)
	}
	var req BookPatchRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	updates := map[string]interface{}{}
	if req.CategoryName.Set {
		if req.CategoryName.Null {
			updates["category_name"] = nil
		} else {
			if utf8.RuneCountInString(strings.TrimSpace(req.CategoryName.Value)) > 100 {
				return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "category_name must be at most 100 characters", nil)
			}
			updates["category_name"] = nullableString(req.CategoryName.Value)
		}
	}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "title cannot be empty", nil)
		}
		updates["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Author != nil {
		updates["author"] = strings.TrimSpace(*req.Author)
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.TotalPages != nil {
		if *req.TotalPages < 1 {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "total_pages must be positive", nil)
		}
		hasRecordedPages, err := h.hasRecordedPagesBeyond(bookID, *req.TotalPages)
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to validate recorded pages", nil)
		}
		if hasRecordedPages {
			return apiError(c, http.StatusConflict, "BOOK_PAGE_COUNT_CONFLICT", "total_pages cannot be lower than saved reading progress or page references", nil)
		}
		updates["total_pages"] = *req.TotalPages
	}
	if req.CoverURL.Set {
		if req.CoverURL.Null {
			updates["cover_url"] = nil
		} else {
			updates["cover_url"] = nullableString(req.CoverURL.Value)
		}
	}
	if req.Publisher.Set {
		if req.Publisher.Null {
			updates["publisher"] = nil
		} else {
			updates["publisher"] = nullableString(req.Publisher.Value)
		}
	}
	if req.PublishedYear.Set {
		if req.PublishedYear.Null {
			updates["published_year"] = nil
		} else {
			updates["published_year"] = req.PublishedYear.Value
		}
	}
	if req.ISBN.Set {
		if req.ISBN.Null {
			updates["isbn"] = nil
		} else {
			updates["isbn"] = nullableString(req.ISBN.Value)
		}
	}
	if len(updates) == 0 {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "At least one field is required", nil)
	}
	updatedBook := book
	if req.Title != nil {
		updatedBook.Title = strings.TrimSpace(*req.Title)
	}
	if req.Author != nil {
		updatedBook.Author = strings.TrimSpace(*req.Author)
	}
	if req.CoverURL.Set {
		updatedBook.CoverURL = nil
		if !req.CoverURL.Null {
			updatedBook.CoverURL = emptyToNil(&req.CoverURL.Value)
		}
	}
	if req.Publisher.Set {
		updatedBook.Publisher = nil
		if !req.Publisher.Null {
			updatedBook.Publisher = emptyToNil(&req.Publisher.Value)
		}
	}
	if req.ISBN.Set {
		updatedBook.ISBN = nil
		if !req.ISBN.Null {
			updatedBook.ISBN = emptyToNil(&req.ISBN.Value)
		}
	}
	if message := validateBookFields(updatedBook); message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	// Clearing or replacing a cover URL also detaches its local file.
	if req.CoverURL.Set && (updatedBook.CoverURL == nil || book.CoverURL == nil || *updatedBook.CoverURL != *book.CoverURL) {
		updates["cover_file"] = nil
	}
	previousCover := copyCoverName(book.CoverFile)
	if err := h.db.Model(&book).Updates(updates).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update book", nil)
	}
	if _, detached := updates["cover_file"]; detached {
		h.removeUnusedCover(c, previousCover)
	}
	if err := h.db.First(&book, "book_id = ?", bookID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reload book", nil)
	}
	return apiSuccess(c, http.StatusOK, book)
}

func (h *BookHandler) ListMine(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	page, limit, err := parsePagination(c)
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
	}
	query := h.libraryQuery(userID)
	if status := c.QueryParam("status"); status != "" {
		if status != statusWantToRead && status != statusReading && status != statusCompleted {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "status must be WANT_TO_READ, READING, or COMPLETED", nil)
		}
		query = query.Where("ub.status = ?", status)
	}
	if favorite := c.QueryParam("favorite"); favorite != "" {
		if favorite != "true" && favorite != "false" {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "favorite must be true or false", nil)
		}
		query = query.Where("ub.is_favorite = ?", favorite == "true")
	}
	if search := strings.TrimSpace(c.QueryParam("search")); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("(b.title ILIKE ? OR b.author ILIKE ?)", pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count library books", nil)
	}
	items := make([]LibraryBook, 0)
	if err := query.Order("ub.added_at DESC").Offset((page - 1) * limit).Limit(limit).Scan(&items).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list library", nil)
	}
	return apiListSuccess(c, items, page, limit, total)
}

func (h *BookHandler) GetMine(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	userBookID, err := parseID(c.Param("user_book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_book_id must be a positive integer", nil)
	}
	item, err := h.findLibraryItem(userID, userBookID)
	if err != nil {
		return lookupError(c, err, "Library book not found", "Failed to load library book")
	}
	return apiSuccess(c, http.StatusOK, item)
}

func (h *BookHandler) AddExisting(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req struct {
		BookID uint64 `json:"book_id"`
	}
	if err := c.Bind(&req); err != nil || req.BookID == 0 || req.BookID > 1<<63-1 {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "book_id is required", nil)
	}
	var book models.Book
	if err := h.visibleBooks(c).Where("book_id = ?", req.BookID).First(&book).Error; err != nil {
		return lookupError(c, err, "Book not found", "Failed to load book")
	}
	userBook := models.UserBook{UserID: userID, BookID: req.BookID, Status: statusWantToRead}
	if err := h.db.Create(&userBook).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apiError(c, http.StatusConflict, "BOOK_ALREADY_IN_LIBRARY", "Book is already in your library", nil)
		}
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to add book", nil)
	}
	item, err := h.findLibraryItem(userID, userBook.UserBookID)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load library book", nil)
	}
	return apiSuccess(c, http.StatusCreated, item)
}

func (h *BookHandler) CreateInLibrary(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req BookRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	book, message := makeBook(req, &userID)
	if message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	userBook := models.UserBook{UserID: userID, Status: statusWantToRead}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&book).Error; err != nil {
			return err
		}
		userBook.BookID = book.BookID
		return tx.Create(&userBook).Error
	})
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create book and add it to library", nil)
	}
	item, err := h.findLibraryItem(userID, userBook.UserBookID)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load library book", nil)
	}
	return apiSuccess(c, http.StatusCreated, item)
}

func (h *BookHandler) UpdateFavorite(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	userBookID, err := parseID(c.Param("user_book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_book_id must be a positive integer", nil)
	}
	var req struct {
		IsFavorite *bool `json:"is_favorite"`
	}
	if err := c.Bind(&req); err != nil || req.IsFavorite == nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "is_favorite is required", nil)
	}
	result := h.db.Model(&models.UserBook{}).Where("user_book_id = ? AND user_id = ?", userBookID, userID).Update("is_favorite", *req.IsFavorite)
	if result.Error != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update favorite", nil)
	}
	if result.RowsAffected == 0 {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	item, err := h.findLibraryItem(userID, userBookID)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reload library book", nil)
	}
	return apiSuccess(c, http.StatusOK, item)
}

func (h *BookHandler) RemoveFromLibrary(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	userBookID, err := parseID(c.Param("user_book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_book_id must be a positive integer", nil)
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var userBook models.UserBook
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_book_id = ? AND user_id = ?", userBookID, userID).First(&userBook).Error; err != nil {
			return err
		}
		var sessions, notes, reviews int64
		if err := tx.Model(&models.ReadingSession{}).Where("user_book_id = ?", userBookID).Count(&sessions).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.BookNote{}).Where("user_book_id = ?", userBookID).Count(&notes).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Review{}).Where("user_book_id = ?", userBookID).Count(&reviews).Error; err != nil {
			return err
		}
		if sessions+notes+reviews > 0 {
			return errHasReadingHistory
		}
		return tx.Delete(&userBook).Error
	})
	if errors.Is(err, errHasReadingHistory) {
		return apiError(c, http.StatusConflict, "BOOK_HAS_HISTORY", "Remove reading sessions, notes, and review before removing this book", nil)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to remove book", nil)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *BookHandler) visibleBooks(c echo.Context) *gorm.DB {
	query := h.db.Model(&models.Book{})
	userID, authenticated := currentUserID(c)
	if !authenticated {
		return query.Where("created_by IS NULL")
	}
	return query.Where("created_by IS NULL OR created_by = ?", userID)
}

func (h *BookHandler) hasRecordedPagesBeyond(bookID uint64, totalPages int) (bool, error) {
	queries := []struct {
		model interface{}
		query string
	}{
		{&models.ReadingSession{}, "user_book_id IN (SELECT user_book_id FROM user_books WHERE book_id = ?) AND current_page > ?"},
		{&models.BookNote{}, "user_book_id IN (SELECT user_book_id FROM user_books WHERE book_id = ?) AND page_number > ?"},
	}
	for _, item := range queries {
		var count int64
		if err := h.db.Model(item.model).Where(item.query, bookID, totalPages).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	var count int64
	if err := h.db.Table("reading_logs AS rl").Joins("JOIN reading_sessions AS rs ON rs.session_id = rl.session_id").
		Joins("JOIN user_books AS ub ON ub.user_book_id = rs.user_book_id").
		Where("ub.book_id = ? AND rl.end_page > ?", bookID, totalPages).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (h *BookHandler) libraryQuery(userID uint64) *gorm.DB {
	return h.db.Table("user_books AS ub").
		Select("ub.user_book_id, b.book_id, b.title, b.author, b.category_name, b.cover_url, b.total_pages, ub.status, ub.is_favorite, COALESCE(rs.current_page, CASE WHEN ub.status = 'COMPLETED' THEN b.total_pages ELSE 0 END) AS current_page, rs.session_id AS active_session_id, CASE WHEN b.total_pages > 0 THEN ROUND(COALESCE(rs.current_page, CASE WHEN ub.status = 'COMPLETED' THEN b.total_pages ELSE 0 END)::numeric * 100 / b.total_pages, 2) ELSE 0 END AS progress_percent").
		Joins("JOIN books AS b ON b.book_id = ub.book_id").
		Joins("LEFT JOIN reading_sessions AS rs ON rs.user_book_id = ub.user_book_id AND rs.status = ?", statusReading).
		Where("ub.user_id = ?", userID)
}

func (h *BookHandler) findLibraryItem(userID, userBookID uint64) (LibraryBook, error) {
	var item LibraryBook
	err := h.libraryQuery(userID).Where("ub.user_book_id = ?", userBookID).Scan(&item).Error
	if err != nil {
		return LibraryBook{}, err
	}
	if item.UserBookID == 0 {
		return LibraryBook{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func makeBook(req BookRequest, createdBy *uint64) (models.Book, string) {
	title, author := strings.TrimSpace(req.Title), strings.TrimSpace(req.Author)
	if title == "" || author == "" {
		return models.Book{}, "title and author are required"
	}
	if req.TotalPages < 1 {
		return models.Book{}, "total_pages must be positive"
	}
	if req.CategoryName != nil && utf8.RuneCountInString(strings.TrimSpace(*req.CategoryName)) > 100 {
		return models.Book{}, "category_name must be at most 100 characters"
	}
	book := models.Book{
		CategoryName:  emptyToNil(req.CategoryName),
		Title:         title,
		Author:        author,
		Description:   req.Description,
		TotalPages:    req.TotalPages,
		CoverURL:      emptyToNil(req.CoverURL),
		Publisher:     emptyToNil(req.Publisher),
		PublishedYear: req.PublishedYear,
		ISBN:          emptyToNil(req.ISBN),
		CreatedBy:     createdBy,
	}
	return book, validateBookFields(book)
}

func emptyToNil(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func nullableString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

var errHasReadingHistory = errors.New("book has reading history")

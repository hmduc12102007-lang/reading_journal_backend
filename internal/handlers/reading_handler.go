package handlers

import (
	"errors"
	"math"
	"net/http"
	"time"

	"reading_journal_backend/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReadingHandler struct {
	db *gorm.DB
}

type SessionResponse struct {
	SessionID     uint64       `json:"session_id"`
	UserBookID    uint64       `json:"user_book_id"`
	SessionNumber int          `json:"session_number"`
	StartDate     models.Date  `json:"start_date"`
	EndDate       *models.Date `json:"end_date"`
	CurrentPage   int          `json:"current_page"`
	TotalPages    int          `json:"total_pages"`
	Status        string       `json:"status"`
	Progress      float64      `json:"progress_percent"`
}

type CreateLogRequest struct {
	SessionID uint64  `json:"session_id"`
	ReadDate  string  `json:"read_date"`
	StartPage int     `json:"start_page"`
	EndPage   int     `json:"end_page"`
	Content   *string `json:"content"`
}

type PatchLogRequest struct {
	ReadDate  *string               `json:"read_date"`
	StartPage *int                  `json:"start_page"`
	EndPage   *int                  `json:"end_page"`
	Content   OptionalField[string] `json:"content"`
}

func NewReadingHandler(db *gorm.DB) *ReadingHandler { return &ReadingHandler{db: db} }

func (h *ReadingHandler) Sessions(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	userBookID, err := parseID(c.Param("user_book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_book_id must be a positive integer", nil)
	}
	var userBook models.UserBook
	if err := h.db.Where("user_book_id = ? AND user_id = ?", userBookID, userID).First(&userBook).Error; err != nil {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	var book models.Book
	if err := h.db.First(&book, "book_id = ?", userBook.BookID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load book", nil)
	}
	sessions := make([]models.ReadingSession, 0)
	if err := h.db.Where("user_book_id = ?", userBookID).Order("session_number DESC").Find(&sessions).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list reading sessions", nil)
	}
	response := make([]SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, makeSessionResponse(session, book.TotalPages))
	}
	return apiSuccess(c, http.StatusOK, response)
}

func (h *ReadingHandler) StartSession(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	userBookID, err := parseID(c.Param("user_book_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_book_id must be a positive integer", nil)
	}
	var req struct {
		StartDate *string `json:"start_date"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	startDate := models.NewDate(time.Now())
	if req.StartDate != nil {
		parsed, err := parseDate(*req.StartDate)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		}
		startDate = models.NewDate(parsed)
	}
	var session models.ReadingSession
	var book models.Book
	created := false
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var userBook models.UserBook
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_book_id = ? AND user_id = ?", userBookID, userID).First(&userBook).Error; err != nil {
			return err
		}
		if err := tx.First(&book, "book_id = ?", userBook.BookID).Error; err != nil {
			return err
		}
		var active models.ReadingSession
		err := tx.Where("user_book_id = ? AND status = ?", userBookID, statusReading).First(&active).Error
		if err == nil {
			session = active
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var latestNumber int
		if err := tx.Model(&models.ReadingSession{}).Where("user_book_id = ?", userBookID).Select("COALESCE(MAX(session_number), 0)").Scan(&latestNumber).Error; err != nil {
			return err
		}
		session = models.ReadingSession{
			UserBookID:    userBookID,
			SessionNumber: latestNumber + 1,
			StartDate:     startDate,
			CurrentPage:   0,
			Status:        statusReading,
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if err := tx.Model(&userBook).Update("status", statusReading).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Library book not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to start reading session", nil)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return apiSuccess(c, status, makeSessionResponse(session, book.TotalPages))
}

func (h *ReadingHandler) UpdateProgress(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	sessionID, err := parseID(c.Param("session_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "session_id must be a positive integer", nil)
	}
	var req struct {
		CurrentPage *int `json:"current_page"`
	}
	if err := c.Bind(&req); err != nil || req.CurrentPage == nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "current_page is required", nil)
	}
	session, book, err := h.loadOwnedSession(userID, sessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading session not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load reading session", nil)
	}
	if session.Status != statusReading {
		return apiError(c, http.StatusConflict, "SESSION_NOT_ACTIVE", "Only an active reading session can be updated", nil)
	}
	if *req.CurrentPage < 0 || *req.CurrentPage > book.TotalPages {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "current_page must be between 0 and total_pages", map[string]string{"current_page": "Must be within the book page range"})
	}
	result := h.db.Model(&models.ReadingSession{}).Where("session_id = ? AND status = ?", sessionID, statusReading).Update("current_page", *req.CurrentPage)
	if result.Error != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update progress", nil)
	}
	if result.RowsAffected == 0 {
		return apiError(c, http.StatusConflict, "SESSION_NOT_ACTIVE", "Only an active reading session can be updated", nil)
	}
	session.CurrentPage = *req.CurrentPage
	return apiSuccess(c, http.StatusOK, makeSessionResponse(session, book.TotalPages))
}

func (h *ReadingHandler) CompleteSession(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	sessionID, err := parseID(c.Param("session_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "session_id must be a positive integer", nil)
	}
	var req struct {
		EndDate *string `json:"end_date"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	endDate := models.NewDate(time.Now())
	if req.EndDate != nil {
		parsed, err := parseDate(*req.EndDate)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		}
		endDate = models.NewDate(parsed)
	}
	var response SessionResponse
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Match StartSession/RemoveFromLibrary lock order: library book, then session.
		var userBook models.UserBook
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND user_book_id = (?)", userID,
			tx.Model(&models.ReadingSession{}).Select("user_book_id").Where("session_id = ?", sessionID)).First(&userBook).Error; err != nil {
			return err
		}
		var session models.ReadingSession
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("session_id = ? AND user_book_id IN (?)", sessionID,
				tx.Model(&models.UserBook{}).Select("user_book_id").Where("user_id = ?", userID)).First(&session).Error
		if err != nil {
			return err
		}
		if session.Status != statusReading {
			return errSessionNotActive
		}
		if endDate.Before(session.StartDate.Time) {
			return errSessionDateInvalid
		}
		var book models.Book
		if err := tx.First(&book, "book_id = ?", userBook.BookID).Error; err != nil {
			return err
		}
		if err := tx.Model(&session).Updates(map[string]interface{}{"status": statusCompleted, "end_date": endDate, "current_page": book.TotalPages}).Error; err != nil {
			return err
		}
		if err := tx.Model(&userBook).Update("status", statusCompleted).Error; err != nil {
			return err
		}
		session.Status = statusCompleted
		session.EndDate = &endDate
		session.CurrentPage = book.TotalPages
		response = makeSessionResponse(session, book.TotalPages)
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading session not found", nil)
	}
	if errors.Is(err, errSessionNotActive) {
		return apiError(c, http.StatusConflict, "SESSION_NOT_ACTIVE", "Reading session is already completed", nil)
	}
	if errors.Is(err, errSessionDateInvalid) {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "end_date must be on or after start_date", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to complete reading session", nil)
	}
	return apiSuccess(c, http.StatusOK, response)
}

func (h *ReadingHandler) ListLogs(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	page, limit, err := parsePagination(c)
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
	}
	ownedSessions := h.db.Model(&models.ReadingSession{}).Select("session_id").Where("user_book_id IN (?)",
		h.db.Model(&models.UserBook{}).Select("user_book_id").Where("user_id = ?", userID))
	query := h.db.Model(&models.ReadingLog{}).Where("session_id IN (?)", ownedSessions)
	if rawID := c.QueryParam("session_id"); rawID != "" {
		sessionID, err := parseID(rawID)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "session_id must be a positive integer", nil)
		}
		query = query.Where("session_id = ?", sessionID)
	}
	for parameter, operator := range map[string]string{"from_date": ">=", "to_date": "<="} {
		if value := c.QueryParam(parameter); value != "" {
			parsed, err := parseDate(value)
			if err != nil {
				return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			}
			query = query.Where("read_date "+operator+" ?", parsed)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count reading logs", nil)
	}
	logs := make([]models.ReadingLog, 0)
	if err := query.Order("read_date DESC, log_id DESC").Offset((page - 1) * limit).Limit(limit).Find(&logs).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list reading logs", nil)
	}
	return apiListSuccess(c, logs, page, limit, total)
}

func (h *ReadingHandler) GetLog(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	logID, err := parseID(c.Param("log_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "log_id must be a positive integer", nil)
	}
	log, err := h.loadOwnedLog(userID, logID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading log not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load reading log", nil)
	}
	return apiSuccess(c, http.StatusOK, log)
}

func (h *ReadingHandler) CreateLog(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req CreateLogRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	if req.SessionID == 0 || req.SessionID > 1<<63-1 {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "session_id is required", nil)
	}
	readDate, err := parseDate(req.ReadDate)
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "read_date must use YYYY-MM-DD format", nil)
	}
	session, book, err := h.loadOwnedSession(userID, req.SessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading session not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load reading session", nil)
	}
	if message := validateLogPages(req.StartPage, req.EndPage, book.TotalPages); message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	log := models.ReadingLog{SessionID: session.SessionID, ReadDate: models.NewDate(readDate), StartPage: req.StartPage, EndPage: req.EndPage, Content: emptyToNil(req.Content)}
	if err := h.db.Create(&log).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create reading log", nil)
	}
	return apiSuccess(c, http.StatusCreated, log)
}

func (h *ReadingHandler) UpdateLog(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	logID, err := parseID(c.Param("log_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "log_id must be a positive integer", nil)
	}
	log, err := h.loadOwnedLog(userID, logID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading log not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load reading log", nil)
	}
	var req PatchLogRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	startPage, endPage := log.StartPage, log.EndPage
	if req.StartPage != nil {
		startPage = *req.StartPage
	}
	if req.EndPage != nil {
		endPage = *req.EndPage
	}
	_, book, err := h.loadOwnedSession(userID, log.SessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading session not found", nil)
	}
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load reading session", nil)
	}
	if message := validateLogPages(startPage, endPage, book.TotalPages); message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	updates := map[string]interface{}{"start_page": startPage, "end_page": endPage}
	if req.ReadDate != nil {
		readDate, err := parseDate(*req.ReadDate)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "read_date must use YYYY-MM-DD format", nil)
		}
		updates["read_date"] = models.NewDate(readDate)
	}
	if req.Content.Set {
		if req.Content.Null {
			updates["content"] = nil
		} else {
			updates["content"] = nullableString(req.Content.Value)
		}
	}
	if err := h.db.Model(&log).Updates(updates).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update reading log", nil)
	}
	log.StartPage, log.EndPage = startPage, endPage
	if req.ReadDate != nil {
		readDate, _ := parseDate(*req.ReadDate)
		log.ReadDate = models.NewDate(readDate)
	}
	if req.Content.Set {
		log.Content = nil
		if !req.Content.Null {
			log.Content = emptyToNil(&req.Content.Value)
		}
	}
	return apiSuccess(c, http.StatusOK, log)
}

func (h *ReadingHandler) DeleteLog(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	logID, err := parseID(c.Param("log_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "log_id must be a positive integer", nil)
	}
	result := h.db.Where("log_id = ? AND session_id IN (?)", logID, h.ownedSessionIDs(userID)).Delete(&models.ReadingLog{})
	if result.Error != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete reading log", nil)
	}
	if result.RowsAffected == 0 {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Reading log not found", nil)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ReadingHandler) loadOwnedSession(userID, sessionID uint64) (models.ReadingSession, models.Book, error) {
	var session models.ReadingSession
	err := h.db.Table("reading_sessions AS rs").Select("rs.*").
		Joins("JOIN user_books AS ub ON ub.user_book_id = rs.user_book_id").
		Where("rs.session_id = ? AND ub.user_id = ?", sessionID, userID).Scan(&session).Error
	if err != nil {
		return session, models.Book{}, err
	}
	if session.SessionID == 0 {
		return session, models.Book{}, gorm.ErrRecordNotFound
	}
	var userBook models.UserBook
	if err := h.db.First(&userBook, "user_book_id = ?", session.UserBookID).Error; err != nil {
		return session, models.Book{}, err
	}
	var book models.Book
	if err := h.db.First(&book, "book_id = ?", userBook.BookID).Error; err != nil {
		return session, models.Book{}, err
	}
	return session, book, nil
}

func (h *ReadingHandler) loadOwnedLog(userID, logID uint64) (models.ReadingLog, error) {
	var log models.ReadingLog
	err := h.db.Where("log_id = ? AND session_id IN (?)", logID, h.ownedSessionIDs(userID)).First(&log).Error
	return log, err
}

func (h *ReadingHandler) ownedSessionIDs(userID uint64) *gorm.DB {
	return h.db.Model(&models.ReadingSession{}).Select("session_id").Where("user_book_id IN (?)",
		h.db.Model(&models.UserBook{}).Select("user_book_id").Where("user_id = ?", userID))
}

func makeSessionResponse(session models.ReadingSession, totalPages int) SessionResponse {
	progress := 0.0
	if totalPages > 0 {
		progress = math.Round(float64(session.CurrentPage)*10000/float64(totalPages)) / 100
	}
	return SessionResponse{
		SessionID: session.SessionID, UserBookID: session.UserBookID, SessionNumber: session.SessionNumber,
		StartDate: session.StartDate, EndDate: session.EndDate, CurrentPage: session.CurrentPage,
		TotalPages: totalPages, Status: session.Status, Progress: progress,
	}
}

func validateLogPages(startPage, endPage, totalPages int) string {
	if startPage < 1 || endPage < startPage || endPage > totalPages {
		return "pages must satisfy 1 <= start_page <= end_page <= total_pages"
	}
	return ""
}

var errSessionNotActive = errors.New("reading session is not active")
var errSessionDateInvalid = errors.New("session end date is before start date")

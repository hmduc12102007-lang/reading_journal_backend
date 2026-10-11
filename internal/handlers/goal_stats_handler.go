package handlers

import (
	"math"
	"net/http"
	"strings"
	"time"

	"reading_journal_backend/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type GoalHandler struct {
	db *gorm.DB
}

type GoalResponse struct {
	GoalID          uint64      `json:"goal_id"`
	GoalName        string      `json:"goal_name"`
	GoalType        string      `json:"goal_type"`
	TargetValue     int         `json:"target_value"`
	StartDate       models.Date `json:"start_date"`
	EndDate         models.Date `json:"end_date"`
	CreatedAt       time.Time   `json:"created_at"`
	CurrentValue    int64       `json:"current_value"`
	ProgressPercent float64     `json:"progress_percent"`
	IsCompleted     bool        `json:"is_completed"`
}

type GoalRequest struct {
	GoalName    string `json:"goal_name"`
	GoalType    string `json:"goal_type"`
	TargetValue int    `json:"target_value"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
}

type GoalPatchRequest struct {
	GoalName    *string `json:"goal_name"`
	GoalType    *string `json:"goal_type"`
	TargetValue *int    `json:"target_value"`
	StartDate   *string `json:"start_date"`
	EndDate     *string `json:"end_date"`
}

type StatisticsResponse struct {
	TotalBooks       int64 `json:"total_books"`
	WantToReadBooks  int64 `json:"want_to_read_books"`
	ReadingBooks     int64 `json:"reading_books"`
	CompletedBooks   int64 `json:"completed_books"`
	TotalPagesRead   int64 `json:"total_pages_read"`
	TotalReadingLogs int64 `json:"total_reading_logs"`
	CompletedGoals   int64 `json:"completed_goals"`
}

func NewGoalHandler(db *gorm.DB) *GoalHandler { return &GoalHandler{db: db} }

func (h *GoalHandler) List(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	page, limit, err := parsePagination(c)
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
	}
	query := h.db.Model(&models.ReadingGoal{}).Where("user_id = ?", userID)
	if goalType := c.QueryParam("goal_type"); goalType != "" {
		if goalType != "BOOKS" && goalType != "PAGES" {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "goal_type must be BOOKS or PAGES", nil)
		}
		query = query.Where("goal_type = ?", goalType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count goals", nil)
	}
	var goals []models.ReadingGoal
	if err := query.Order("start_date DESC, goal_id DESC").Offset((page - 1) * limit).Limit(limit).Find(&goals).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list goals", nil)
	}
	items := make([]GoalResponse, 0, len(goals))
	for _, goal := range goals {
		item, err := h.withProgress(userID, goal)
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to calculate goal progress", nil)
		}
		items = append(items, item)
	}
	return apiListSuccess(c, items, page, limit, total)
}

func (h *GoalHandler) Get(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	goalID, err := parseID(c.Param("goal_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "goal_id must be a positive integer", nil)
	}
	var goal models.ReadingGoal
	if err := h.db.Where("goal_id = ? AND user_id = ?", goalID, userID).First(&goal).Error; err != nil {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Goal not found", nil)
	}
	item, err := h.withProgress(userID, goal)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to calculate goal progress", nil)
	}
	return apiSuccess(c, http.StatusOK, item)
}

func (h *GoalHandler) Create(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req GoalRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	goal, message := buildGoal(userID, req)
	if message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	if err := h.db.Create(&goal).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create goal", nil)
	}
	item, err := h.withProgress(userID, goal)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to calculate goal progress", nil)
	}
	return apiSuccess(c, http.StatusCreated, item)
}

func (h *GoalHandler) Update(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	goalID, err := parseID(c.Param("goal_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "goal_id must be a positive integer", nil)
	}
	var goal models.ReadingGoal
	if err := h.db.Where("goal_id = ? AND user_id = ?", goalID, userID).First(&goal).Error; err != nil {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Goal not found", nil)
	}
	var req GoalPatchRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	if req.GoalName == nil && req.GoalType == nil && req.TargetValue == nil && req.StartDate == nil && req.EndDate == nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "At least one field is required", nil)
	}
	updated := goal
	if req.GoalName != nil {
		updated.GoalName = *req.GoalName
	}
	if req.GoalType != nil {
		updated.GoalType = *req.GoalType
	}
	if req.TargetValue != nil {
		updated.TargetValue = *req.TargetValue
	}
	if req.StartDate != nil {
		parsed, err := parseDate(*req.StartDate)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "start_date must use YYYY-MM-DD format", nil)
		}
		updated.StartDate = models.NewDate(parsed)
	}
	if req.EndDate != nil {
		parsed, err := parseDate(*req.EndDate)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "end_date must use YYYY-MM-DD format", nil)
		}
		updated.EndDate = models.NewDate(parsed)
	}
	if message := validateGoal(updated); message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}
	if err := h.db.Model(&goal).Updates(map[string]interface{}{
		"goal_name": updated.GoalName, "goal_type": updated.GoalType, "target_value": updated.TargetValue,
		"start_date": updated.StartDate, "end_date": updated.EndDate,
	}).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update goal", nil)
	}
	updated.GoalID = goal.GoalID
	updated.UserID = userID
	updated.CreatedAt = goal.CreatedAt
	item, err := h.withProgress(userID, updated)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to calculate goal progress", nil)
	}
	return apiSuccess(c, http.StatusOK, item)
}

func (h *GoalHandler) Delete(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	goalID, err := parseID(c.Param("goal_id"))
	if err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "goal_id must be a positive integer", nil)
	}
	result := h.db.Where("goal_id = ? AND user_id = ?", goalID, userID).Delete(&models.ReadingGoal{})
	if result.Error != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete goal", nil)
	}
	if result.RowsAffected == 0 {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", "Goal not found", nil)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *GoalHandler) Dashboard(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	stats, err := h.statistics(userID, nil, nil)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load dashboard", nil)
	}
	reading := make([]LibraryBook, 0, 5)
	if err := h.db.Table("user_books AS ub").Select("ub.user_book_id, b.book_id, b.title, b.author, b.category_name, b.cover_url, b.total_pages, ub.status, ub.is_favorite, COALESCE(rs.current_page, 0) AS current_page, rs.session_id AS active_session_id, CASE WHEN b.total_pages > 0 THEN ROUND(COALESCE(rs.current_page, 0)::numeric * 100 / b.total_pages, 2) ELSE 0 END AS progress_percent").
		Joins("JOIN books AS b ON b.book_id = ub.book_id").Joins("LEFT JOIN reading_sessions AS rs ON rs.user_book_id = ub.user_book_id AND rs.status = ?", statusReading).
		Where("ub.user_id = ? AND ub.status = ?", userID, statusReading).Order("ub.added_at DESC").Limit(5).Scan(&reading).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load current reading books", nil)
	}
	if reading == nil {
		reading = make([]LibraryBook, 0)
	}
	return apiSuccess(c, http.StatusOK, echo.Map{"statistics": stats, "current_reading": reading})
}

func (h *GoalHandler) Statistics(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var fromDate, toDate *time.Time
	if value := c.QueryParam("from_date"); value != "" {
		parsed, err := parseDate(value)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "from_date must use YYYY-MM-DD format", nil)
		}
		fromDate = &parsed
	}
	if value := c.QueryParam("to_date"); value != "" {
		parsed, err := parseDate(value)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "to_date must use YYYY-MM-DD format", nil)
		}
		toDate = &parsed
	}
	if fromDate != nil && toDate != nil && toDate.Before(*fromDate) {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "to_date must be on or after from_date", nil)
	}
	stats, err := h.statistics(userID, fromDate, toDate)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load statistics", nil)
	}
	return apiSuccess(c, http.StatusOK, stats)
}

func (h *GoalHandler) statistics(userID uint64, fromDate, toDate *time.Time) (StatisticsResponse, error) {
	var response StatisticsResponse
	if err := h.db.Model(&models.UserBook{}).Where("user_id = ?", userID).Count(&response.TotalBooks).Error; err != nil {
		return response, err
	}
	for status, target := range map[string]*int64{
		statusWantToRead: &response.WantToReadBooks,
		statusReading:    &response.ReadingBooks,
		statusCompleted:  &response.CompletedBooks,
	} {
		if err := h.db.Model(&models.UserBook{}).Where("user_id = ? AND status = ?", userID, status).Count(target).Error; err != nil {
			return response, err
		}
	}
	logQuery := h.db.Table("reading_logs AS rl").Select("COALESCE(SUM(rl.end_page - rl.start_page + 1), 0)").
		Joins("JOIN reading_sessions AS rs ON rs.session_id = rl.session_id").
		Joins("JOIN user_books AS ub ON ub.user_book_id = rs.user_book_id").Where("ub.user_id = ?", userID)
	countQuery := h.db.Table("reading_logs AS rl").Joins("JOIN reading_sessions AS rs ON rs.session_id = rl.session_id").
		Joins("JOIN user_books AS ub ON ub.user_book_id = rs.user_book_id").Where("ub.user_id = ?", userID)
	if fromDate != nil {
		logQuery = logQuery.Where("rl.read_date >= ?", *fromDate)
		countQuery = countQuery.Where("rl.read_date >= ?", *fromDate)
	}
	if toDate != nil {
		logQuery = logQuery.Where("rl.read_date <= ?", *toDate)
		countQuery = countQuery.Where("rl.read_date <= ?", *toDate)
	}
	if err := logQuery.Scan(&response.TotalPagesRead).Error; err != nil {
		return response, err
	}
	if err := countQuery.Count(&response.TotalReadingLogs).Error; err != nil {
		return response, err
	}
	var goals []models.ReadingGoal
	if err := h.db.Where("user_id = ?", userID).Find(&goals).Error; err != nil {
		return response, err
	}
	for _, goal := range goals {
		progress, err := h.goalValue(userID, goal)
		if err != nil {
			return response, err
		}
		if progress >= int64(goal.TargetValue) {
			response.CompletedGoals++
		}
	}
	return response, nil
}

func (h *GoalHandler) withProgress(userID uint64, goal models.ReadingGoal) (GoalResponse, error) {
	current, err := h.goalValue(userID, goal)
	if err != nil {
		return GoalResponse{}, err
	}
	percent := math.Round(float64(current)*10000/float64(goal.TargetValue)) / 100
	if percent > 100 {
		percent = 100
	}
	return GoalResponse{
		GoalID: goal.GoalID, GoalName: goal.GoalName, GoalType: goal.GoalType, TargetValue: goal.TargetValue,
		StartDate: goal.StartDate, EndDate: goal.EndDate, CreatedAt: goal.CreatedAt,
		CurrentValue: current, ProgressPercent: percent, IsCompleted: current >= int64(goal.TargetValue),
	}, nil
}

func (h *GoalHandler) goalValue(userID uint64, goal models.ReadingGoal) (int64, error) {
	var value int64
	if goal.GoalType == "BOOKS" {
		err := h.db.Table("reading_sessions AS rs").Select("COUNT(DISTINCT b.book_id)").
			Joins("JOIN user_books AS ub ON ub.user_book_id = rs.user_book_id").
			Joins("JOIN books AS b ON b.book_id = ub.book_id").
			Where("ub.user_id = ? AND rs.status = ? AND rs.end_date >= ? AND rs.end_date <= ?", userID, statusCompleted, goal.StartDate, goal.EndDate).
			Scan(&value).Error
		return value, err
	}
	err := h.db.Table("reading_logs AS rl").Select("COALESCE(SUM(rl.end_page - rl.start_page + 1), 0)").
		Joins("JOIN reading_sessions AS rs ON rs.session_id = rl.session_id").
		Joins("JOIN user_books AS ub ON ub.user_book_id = rs.user_book_id").
		Where("ub.user_id = ? AND rl.read_date >= ? AND rl.read_date <= ?", userID, goal.StartDate, goal.EndDate).
		Scan(&value).Error
	return value, err
}

func buildGoal(userID uint64, req GoalRequest) (models.ReadingGoal, string) {
	start, err := parseDate(req.StartDate)
	if err != nil {
		return models.ReadingGoal{}, "start_date must use YYYY-MM-DD format"
	}
	end, err := parseDate(req.EndDate)
	if err != nil {
		return models.ReadingGoal{}, "end_date must use YYYY-MM-DD format"
	}
	goal := models.ReadingGoal{UserID: userID, GoalName: strings.TrimSpace(req.GoalName), GoalType: req.GoalType, TargetValue: req.TargetValue, StartDate: models.NewDate(start), EndDate: models.NewDate(end)}
	return goal, validateGoal(goal)
}

func validateGoal(goal models.ReadingGoal) string {
	if goal.GoalName == "" {
		return "goal_name is required"
	}
	if goal.GoalType != "BOOKS" && goal.GoalType != "PAGES" {
		return "goal_type must be BOOKS or PAGES"
	}
	if goal.TargetValue <= 0 {
		return "target_value must be positive"
	}
	if goal.EndDate.Before(goal.StartDate.Time) {
		return "end_date must be on or after start_date"
	}
	return ""
}

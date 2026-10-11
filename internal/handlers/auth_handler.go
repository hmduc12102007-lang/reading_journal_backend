package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"reading_journal_backend/internal/auth"
	"reading_journal_backend/internal/models"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db        *gorm.DB
	jwtSecret string
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type UserResponse struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type AuthResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	User        UserResponse `json:"user"`
}

func NewAuthHandler(db *gorm.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret}
}

func (h *AuthHandler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || utf8.RuneCountInString(req.Username) > 100 {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "username is required and must be at most 100 characters", nil)
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "A valid email is required", nil)
	}
	if message := validatePassword(req.Password); message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to secure password", nil)
	}

	user := models.User{Username: req.Username, Email: email, PasswordHash: string(hash)}
	if err := h.db.Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apiError(c, http.StatusConflict, "EMAIL_ALREADY_EXISTS", "Email is already registered", nil)
		}
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to register user", nil)
	}

	token, err := auth.IssueToken(user, h.jwtSecret)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create access token", nil)
	}
	return apiSuccess(c, http.StatusCreated, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        toUserResponse(user),
	})
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	email, ok := normalizeEmail(req.Email)
	if !ok || req.Password == "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Valid email and password are required", nil)
	}

	var user models.User
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apiError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect", nil)
		}
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to sign in", nil)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return apiError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect", nil)
	}

	token, err := auth.IssueToken(user, h.jwtSecret)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create access token", nil)
	}
	return apiSuccess(c, http.StatusOK, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        toUserResponse(user),
	})
}

func (h *AuthHandler) GetMe(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var user models.User
	if err := h.db.First(&user, "user_id = ?", userID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load profile", nil)
	}
	return apiSuccess(c, http.StatusOK, toUserResponse(user))
}

func (h *AuthHandler) UpdateMe(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req struct {
		Username *string `json:"username"`
	}
	if err := c.Bind(&req); err != nil || req.Username == nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "username is required", nil)
	}
	username := strings.TrimSpace(*req.Username)
	if username == "" || utf8.RuneCountInString(username) > 100 {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "username must contain 1 to 100 characters", nil)
	}
	if err := h.db.Model(&models.User{}).Where("user_id = ?", userID).Update("username", username).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update profile", nil)
	}
	var user models.User
	if err := h.db.First(&user, "user_id = ?", userID).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load profile", nil)
	}
	return apiSuccess(c, http.StatusOK, toUserResponse(user))
}

func (h *AuthHandler) ChangePassword(c echo.Context) error {
	userID, ok := currentUserID(c)
	if !ok {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body", nil)
	}
	if req.CurrentPassword == "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "current_password is required", nil)
	}
	if message := validatePassword(req.NewPassword); message != "" {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", message, nil)
	}

	var user models.User
	if err := h.db.First(&user, "user_id = ?", userID).Error; err != nil {
		return apiError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		return apiError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Current password is incorrect", nil)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.NewPassword)) == nil {
		return apiError(c, http.StatusBadRequest, "VALIDATION_ERROR", "New password must differ from current password", nil)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to secure password", nil)
	}
	if err := h.db.Model(&models.User{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"password_hash": string(hash),
		"token_version": gorm.Expr("token_version + ?", 1),
	}).Error; err != nil {
		return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to change password", nil)
	}
	return apiSuccessMessage(c, http.StatusOK, "Password changed", nil)
}

func toUserResponse(user models.User) UserResponse {
	return UserResponse{UserID: user.UserID, Username: user.Username, Email: user.Email}
}

func normalizeEmail(value string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(value))
	if len(email) > 254 {
		return "", false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", false
	}
	return email, true
}

func validatePassword(password string) string {
	if utf8.RuneCountInString(password) < 8 {
		return "password must be at least 8 characters"
	}
	if len(password) > 72 {
		return "password must be at most 72 bytes"
	}
	return ""
}

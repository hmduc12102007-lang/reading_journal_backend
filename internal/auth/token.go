package auth

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"reading_journal_backend/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const UserIDContextKey = "auth_user_id"

const tokenIssuer = "reading-journal-backend"

const accessTokenLifetime = 24 * time.Hour

type Claims struct {
	UserID       uint64 `json:"user_id"`
	TokenVersion uint64 `json:"token_version"`
	jwt.RegisteredClaims
}

func IssueToken(user models.User, secret string) (string, error) {
	now := time.Now()
	expiresAt := now.Add(accessTokenLifetime)
	claims := Claims{
		UserID:       user.UserID,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   strconv.FormatUint(user.UserID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	return signed, err
}

func Middleware(db *gorm.DB, secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			parts := strings.Fields(c.Request().Header.Get(echo.HeaderAuthorization))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return unauthorized(c, "Bearer token is required")
			}

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
				if token.Method == nil || token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, errors.New("unexpected signing algorithm")
				}
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(tokenIssuer))
			if err != nil || token == nil || !token.Valid || claims.UserID == 0 {
				return unauthorized(c, "Invalid or expired token")
			}

			var user models.User
			err = db.Select("user_id", "token_version").First(&user, "user_id = ?", claims.UserID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return unauthorized(c, "Invalid or expired token")
			}
			if err != nil {
				return c.JSON(http.StatusInternalServerError, echo.Map{"success": false, "error": echo.Map{"code": "INTERNAL_ERROR", "message": "Failed to validate token"}})
			}
			if user.TokenVersion != claims.TokenVersion {
				return unauthorized(c, "Invalid or expired token")
			}

			c.Set(UserIDContextKey, user.UserID)
			return next(c)
		}
	}
}

func unauthorized(c echo.Context, message string) error {
	return c.JSON(http.StatusUnauthorized, echo.Map{
		"success": false,
		"error":   echo.Map{"code": "UNAUTHORIZED", "message": message},
	})
}

func OptionalMiddleware(db *gorm.DB, secret string) echo.MiddlewareFunc {
	required := Middleware(db, secret)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if strings.TrimSpace(c.Request().Header.Get(echo.HeaderAuthorization)) == "" {
				return next(c)
			}
			return required(next)(c)
		}
	}
}

package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func TestRejectInvalidTokenBeforeDatabaseAccess(t *testing.T) {
	secret := strings.Repeat("secret", 8)
	for _, tc := range []struct {
		name   string
		expiry *jwt.NumericDate
		issuer string
	}{
		{"missing expiry", nil, tokenIssuer},
		{"expired", jwt.NewNumericDate(time.Now().Add(-time.Hour)), tokenIssuer},
		{"wrong issuer", jwt.NewNumericDate(time.Now().Add(time.Hour)), "wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{UserID: 1, RegisteredClaims: jwt.RegisteredClaims{Issuer: tc.issuer, ExpiresAt: tc.expiry}}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			e := echo.New()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			err = Middleware(nil, secret)(func(c echo.Context) error { t.Fatal("invalid token reached handler"); return nil })(e.NewContext(req, rec))
			if err != nil || rec.Code != 401 {
				t.Fatalf("code %d, error %v", rec.Code, err)
			}
		})
	}
}

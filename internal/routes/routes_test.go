package routes_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"reading_journal_backend/internal/routes"

	"github.com/labstack/echo/v4"
)

func TestCoverRouteMethodsAndPathParameter(t *testing.T) {
	e := echo.New()
	pass := func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	routes.Register(e, routes.Dependencies{AuthMiddleware: pass, OptionalAuth: pass})
	// Observe the router's actual result before handlers access the database.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return c.JSON(200, map[string]string{"route": c.Path(), "book_id": c.Param("book_id")})
		}
	})
	for _, method := range []string{"POST", "GET", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/books/2/cover", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			var result map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || result["route"] != "/api/v1/books/:book_id/cover" || result["book_id"] != "2" {
				t.Fatalf("route or parameter mismatch: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

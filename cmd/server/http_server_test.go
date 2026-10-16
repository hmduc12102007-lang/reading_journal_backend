package main

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHTTPErrorResponses(t *testing.T) {
	e := newHTTPServer()
	e.Logger.SetOutput(io.Discard)
	e.POST("/body", func(c echo.Context) error {
		var body map[string]interface{}
		if err := c.Bind(&body); err != nil {
			return err
		}
		return c.NoContent(204)
	})
	e.GET("/failure", func(c echo.Context) error { return errors.New("private database connection details") })
	for _, tc := range []struct {
		name, method, path, body, contentType string
		status                                int
		code                                  string
	}{
		{"body too large", "POST", "/body", strings.Repeat("x", (6<<20)+1), "application/json", 413, "PAYLOAD_TOO_LARGE"},
		{"invalid JSON", "POST", "/body", "{", "application/json", 400, "VALIDATION_ERROR"},
		{"unsupported media", "POST", "/body", "hello", "application/octet-stream", 415, "UNSUPPORTED_MEDIA_TYPE"},
		{"missing route", "GET", "/missing", "", "", 404, "NOT_FOUND"},
		{"wrong method", "DELETE", "/body", "", "", 405, "METHOD_NOT_ALLOWED"},
		{"internal error", "GET", "/failure", "", "", 500, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.code) || strings.Contains(rec.Body.String(), "private database") {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
	if e.Server.ReadHeaderTimeout == 0 || e.Server.ReadTimeout == 0 || e.Server.WriteTimeout == 0 || e.Server.IdleTimeout == 0 {
		t.Fatal("missing HTTP timeouts")
	}
}

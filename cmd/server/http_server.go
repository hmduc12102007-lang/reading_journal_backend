package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func newHTTPServer() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.ReadTimeout = 30 * time.Second
	e.Server.WriteTimeout = 30 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		status := http.StatusInternalServerError
		var httpError *echo.HTTPError
		if errors.As(err, &httpError) {
			status = httpError.Code
		}
		code := "INTERNAL_ERROR"
		message := "Internal server error"
		if status == http.StatusNotFound {
			code, message = "NOT_FOUND", "Resource not found"
		} else if status == http.StatusMethodNotAllowed {
			code, message = "METHOD_NOT_ALLOWED", "Method not allowed"
		} else if status >= 400 && status < 500 {
			message = http.StatusText(status)
			switch status {
			case http.StatusRequestEntityTooLarge:
				code = "PAYLOAD_TOO_LARGE"
			case http.StatusTooManyRequests:
				code = "RATE_LIMIT_EXCEEDED"
			case http.StatusUnsupportedMediaType:
				code = "UNSUPPORTED_MEDIA_TYPE"
			default:
				code = "VALIDATION_ERROR"
			}
		}
		_ = c.JSON(status, echo.Map{"success": false, "error": echo.Map{"code": code, "message": message}})
	}
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("6M"))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
		},
	}))

	return e
}

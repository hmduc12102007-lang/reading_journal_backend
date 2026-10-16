package handlers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func lookupError(c echo.Context, err error, missing, failure string) error {
	var httpError *echo.HTTPError
	if errors.As(err, &httpError) {
		code := "VALIDATION_ERROR"
		if httpError.Code == http.StatusUnauthorized {
			code = "UNAUTHORIZED"
		}
		return apiError(c, httpError.Code, code, http.StatusText(httpError.Code), nil)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apiError(c, http.StatusNotFound, "NOT_FOUND", missing, nil)
	}
	return apiError(c, http.StatusInternalServerError, "INTERNAL_ERROR", failure, nil)
}

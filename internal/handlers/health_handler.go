package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func Health(c echo.Context) error {
	return apiSuccess(c, http.StatusOK, echo.Map{"status": "ok"})
}

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

type OptionalField[T any] struct {
	Set   bool
	Null  bool
	Value T
}

func (field *OptionalField[T]) UnmarshalJSON(data []byte) error {
	field.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		field.Null = true
		return nil
	}
	return json.Unmarshal(data, &field.Value)
}

type Pagination struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

func apiSuccess(c echo.Context, status int, data interface{}) error {
	return c.JSON(status, echo.Map{"success": true, "data": data})
}

func apiSuccessMessage(c echo.Context, status int, message string, data interface{}) error {
	return c.JSON(status, echo.Map{"success": true, "message": message, "data": data})
}

func apiListSuccess(c echo.Context, data interface{}, page, limit int, total int64) error {
	return c.JSON(http.StatusOK, echo.Map{
		"success":    true,
		"data":       data,
		"pagination": Pagination{Page: page, Limit: limit, Total: total},
	})
}

func apiError(c echo.Context, status int, code, message string, details interface{}) error {
	err := echo.Map{"code": code, "message": message}
	if details != nil {
		err["details"] = details
	}
	return c.JSON(status, echo.Map{"success": false, "error": err})
}

func parseID(value string) (uint64, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func parsePagination(c echo.Context) (int, int, error) {
	page, limit := 1, 20
	var err error
	if value := c.QueryParam("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 {
			return 0, 0, fmt.Errorf("page must be a positive integer")
		}
	}
	if value := c.QueryParam("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit, nil
}

func currentUserID(c echo.Context) (uint64, bool) {
	userID, ok := c.Get("auth_user_id").(uint64)
	return userID, ok && userID != 0
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must use YYYY-MM-DD format")
	}
	return parsed, nil
}

package main

import (
	"errors"
	"log"
	"net/http"

	"reading_journal_backend/internal/auth"
	"reading_journal_backend/internal/config"
	"reading_journal_backend/internal/database"
	"reading_journal_backend/internal/handlers"
	"reading_journal_backend/internal/routes"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := config.Load()
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	e := echo.New()
	e.HideBanner = true
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
		}
		_ = c.JSON(status, echo.Map{"success": false, "error": echo.Map{"code": code, "message": message}})
	}
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
		},
	}))

	authHandler := handlers.NewAuthHandler(db, cfg.JWTSecret)
	bookHandler := handlers.NewBookHandler(db)
	readingHandler := handlers.NewReadingHandler(db)
	notesReviewsHandler := handlers.NewNotesReviewsHandler(db)
	goalHandler := handlers.NewGoalHandler(db)
	routes.Register(e, routes.Dependencies{
		AuthHandler:         authHandler,
		BookHandler:         bookHandler,
		ReadingHandler:      readingHandler,
		NotesReviewsHandler: notesReviewsHandler,
		GoalHandler:         goalHandler,
		AuthMiddleware:      auth.Middleware(db, cfg.JWTSecret),
		OptionalAuth:        auth.OptionalMiddleware(db, cfg.JWTSecret),
	})

	if err := e.Start(":" + cfg.AppPort); err != nil {
		log.Fatal(err)
	}
}

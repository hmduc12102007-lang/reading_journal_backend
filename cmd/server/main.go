package main

import (
	"log"
	"os"

	"reading_journal_backend/internal/auth"
	"reading_journal_backend/internal/config"
	"reading_journal_backend/internal/database"
	"reading_journal_backend/internal/handlers"
	"reading_journal_backend/internal/routes"
)

func main() {
	cfg := config.Load()
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}
	if err := os.MkdirAll(cfg.UploadDir, 0700); err != nil {
		log.Fatalf("create image storage: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	e := newHTTPServer()

	authHandler := handlers.NewAuthHandler(db, cfg.JWTSecret)
	bookHandler := handlers.NewBookHandler(db, cfg.UploadDir)
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

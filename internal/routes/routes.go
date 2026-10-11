package routes

import (
	"reading_journal_backend/internal/handlers"

	"github.com/labstack/echo/v4"
)

type Dependencies struct {
	AuthHandler         *handlers.AuthHandler
	BookHandler         *handlers.BookHandler
	ReadingHandler      *handlers.ReadingHandler
	NotesReviewsHandler *handlers.NotesReviewsHandler
	GoalHandler         *handlers.GoalHandler
	AuthMiddleware      echo.MiddlewareFunc
	OptionalAuth        echo.MiddlewareFunc
}

func Register(e *echo.Echo, deps Dependencies) {
	e.GET("/health", handlers.Health)

	api := e.Group("/api/v1")
	auth := api.Group("/auth")
	auth.POST("/register", deps.AuthHandler.Register)
	auth.POST("/login", deps.AuthHandler.Login)
	auth.PATCH("/change-password", deps.AuthHandler.ChangePassword, deps.AuthMiddleware)

	users := api.Group("/users", deps.AuthMiddleware)
	users.GET("/me", deps.AuthHandler.GetMe)
	users.PATCH("/me", deps.AuthHandler.UpdateMe)

	books := api.Group("/books")
	books.GET("", deps.BookHandler.List, deps.OptionalAuth)
	books.GET("/:book_id", deps.BookHandler.Get, deps.OptionalAuth)
	books.POST("", deps.BookHandler.Create, deps.AuthMiddleware)
	books.PATCH("/:book_id", deps.BookHandler.Update, deps.AuthMiddleware)

	meBooks := api.Group("/me/books", deps.AuthMiddleware)
	meBooks.GET("", deps.BookHandler.ListMine)
	meBooks.GET("/:user_book_id", deps.BookHandler.GetMine)
	meBooks.POST("", deps.BookHandler.AddExisting)
	meBooks.POST("/new", deps.BookHandler.CreateInLibrary)
	meBooks.PATCH("/:user_book_id", deps.BookHandler.UpdateFavorite)
	meBooks.DELETE("/:user_book_id", deps.BookHandler.RemoveFromLibrary)
	meBooks.GET("/:user_book_id/sessions", deps.ReadingHandler.Sessions)
	meBooks.POST("/:user_book_id/sessions", deps.ReadingHandler.StartSession)
	meBooks.GET("/:user_book_id/notes", deps.NotesReviewsHandler.ListNotes)
	meBooks.POST("/:user_book_id/notes", deps.NotesReviewsHandler.CreateNote)
	meBooks.GET("/:user_book_id/review", deps.NotesReviewsHandler.GetReview)
	meBooks.PUT("/:user_book_id/review", deps.NotesReviewsHandler.UpsertReview)
	meBooks.DELETE("/:user_book_id/review", deps.NotesReviewsHandler.DeleteReview)

	sessions := api.Group("/sessions", deps.AuthMiddleware)
	sessions.PATCH("/:session_id/progress", deps.ReadingHandler.UpdateProgress)
	sessions.POST("/:session_id/complete", deps.ReadingHandler.CompleteSession)

	logs := api.Group("/reading-logs", deps.AuthMiddleware)
	logs.GET("", deps.ReadingHandler.ListLogs)
	logs.POST("", deps.ReadingHandler.CreateLog)
	logs.GET("/:log_id", deps.ReadingHandler.GetLog)
	logs.PATCH("/:log_id", deps.ReadingHandler.UpdateLog)
	logs.DELETE("/:log_id", deps.ReadingHandler.DeleteLog)

	notes := api.Group("/notes", deps.AuthMiddleware)
	notes.PATCH("/:note_id", deps.NotesReviewsHandler.UpdateNote)
	notes.DELETE("/:note_id", deps.NotesReviewsHandler.DeleteNote)

	goals := api.Group("/goals", deps.AuthMiddleware)
	goals.GET("", deps.GoalHandler.List)
	goals.POST("", deps.GoalHandler.Create)
	goals.GET("/:goal_id", deps.GoalHandler.Get)
	goals.PATCH("/:goal_id", deps.GoalHandler.Update)
	goals.DELETE("/:goal_id", deps.GoalHandler.Delete)

	api.GET("/dashboard", deps.GoalHandler.Dashboard, deps.AuthMiddleware)
	api.GET("/statistics", deps.GoalHandler.Statistics, deps.AuthMiddleware)
}

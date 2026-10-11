package database

import (
	"fmt"

	"reading_journal_backend/internal/config"
	"reading_journal_backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect(cfg config.Config) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(cfg.DBConnectionString), &gorm.Config{TranslateError: true})
}

func AutoMigrate(db *gorm.DB) error {
	var categoryNameColumnExists bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'books' AND column_name = 'category_name'
		)
	`).Scan(&categoryNameColumnExists).Error; err != nil {
		return fmt.Errorf("check book category_name column: %w", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Book{},
		&models.UserBook{},
		&models.ReadingSession{},
		&models.ReadingLog{},
		&models.BookNote{},
		&models.Review{},
		&models.ReadingGoal{},
	); err != nil {
		return err
	}
	if !categoryNameColumnExists {
		if err := migrateLegacyCategoryNames(db); err != nil {
			return err
		}
	}
	return migrateForeignKeys(db)
}

func migrateLegacyCategoryNames(db *gorm.DB) error {
	var hasLegacySchema bool
	if err := db.Raw(`
		SELECT
			EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'books' AND column_name = 'category_id')
			AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'categories' AND column_name = 'category_id')
			AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'categories' AND column_name = 'category_name')
	`).Scan(&hasLegacySchema).Error; err != nil {
		return fmt.Errorf("check legacy category schema: %w", err)
	}
	if !hasLegacySchema {
		return nil
	}
	if err := db.Exec(`
		UPDATE books AS b
		SET category_name = c.category_name
		FROM categories AS c
		WHERE b.category_id = c.category_id AND b.category_name IS NULL
	`).Error; err != nil {
		return fmt.Errorf("copy legacy category names: %w", err)
	}
	return nil
}

func migrateForeignKeys(db *gorm.DB) error {
	constraints := []struct {
		table string
		name  string
		ddl   string
	}{
		{"books", "fk_books_creator", "FOREIGN KEY (created_by) REFERENCES users(user_id)"},
		{"user_books", "fk_user_books_user", "FOREIGN KEY (user_id) REFERENCES users(user_id)"},
		{"user_books", "fk_user_books_book", "FOREIGN KEY (book_id) REFERENCES books(book_id)"},
		{"reading_sessions", "fk_reading_sessions_user_book", "FOREIGN KEY (user_book_id) REFERENCES user_books(user_book_id)"},
		{"reading_logs", "fk_reading_logs_session", "FOREIGN KEY (session_id) REFERENCES reading_sessions(session_id)"},
		{"book_notes", "fk_book_notes_user_book", "FOREIGN KEY (user_book_id) REFERENCES user_books(user_book_id)"},
		{"reviews", "fk_reviews_user_book", "FOREIGN KEY (user_book_id) REFERENCES user_books(user_book_id)"},
		{"reading_goals", "fk_reading_goals_user", "FOREIGN KEY (user_id) REFERENCES users(user_id)"},
	}
	for _, constraint := range constraints {
		var exists bool
		if err := db.Raw(
			"SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = ? AND conrelid = to_regclass(?))",
			constraint.name,
			constraint.table,
		).Scan(&exists).Error; err != nil {
			return fmt.Errorf("check foreign key %s: %w", constraint.name, err)
		}
		if exists {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s", constraint.table, constraint.name, constraint.ddl)
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("create foreign key %s: %w", constraint.name, err)
		}
	}
	return nil
}

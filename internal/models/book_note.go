package models

import "time"

type BookNote struct {
	NoteID     uint64    `json:"note_id" gorm:"primaryKey;column:note_id"`
	UserBookID uint64    `json:"user_book_id" gorm:"not null;index"`
	PageNumber *int      `json:"page_number"`
	Content    string    `json:"content" gorm:"type:text;not null"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (BookNote) TableName() string { return "book_notes" }

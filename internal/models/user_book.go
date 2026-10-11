package models

import "time"

type UserBook struct {
	UserBookID uint64    `json:"user_book_id" gorm:"primaryKey;column:user_book_id"`
	UserID     uint64    `json:"user_id" gorm:"not null;uniqueIndex:ux_user_books_user_book"`
	BookID     uint64    `json:"book_id" gorm:"not null;uniqueIndex:ux_user_books_user_book"`
	Status     string    `json:"status" gorm:"size:32;not null;default:WANT_TO_READ"`
	IsFavorite bool      `json:"is_favorite" gorm:"not null;default:false"`
	AddedAt    time.Time `json:"added_at" gorm:"autoCreateTime"`
}

func (UserBook) TableName() string { return "user_books" }

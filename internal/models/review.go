package models

import "time"

type Review struct {
	ReviewID   uint64    `json:"review_id" gorm:"primaryKey;column:review_id"`
	UserBookID uint64    `json:"user_book_id" gorm:"not null;uniqueIndex"`
	Rating     int       `json:"rating" gorm:"not null"`
	Comment    *string   `json:"comment" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Review) TableName() string { return "reviews" }

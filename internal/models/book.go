package models

import "time"

type Book struct {
	BookID        uint64    `json:"book_id" gorm:"primaryKey;column:book_id"`
	CategoryName  *string   `json:"category_name" gorm:"size:100"`
	Title         string    `json:"title" gorm:"size:255;not null;index"`
	Author        string    `json:"author" gorm:"size:255;not null"`
	Description   string    `json:"description" gorm:"type:text"`
	TotalPages    int       `json:"total_pages" gorm:"not null"`
	CoverURL      *string   `json:"cover_url" gorm:"size:2048"`
	Publisher     *string   `json:"publisher" gorm:"size:255"`
	PublishedYear *int      `json:"published_year"`
	ISBN          *string   `json:"isbn" gorm:"size:32;index"`
	CreatedBy     *uint64   `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

func (Book) TableName() string { return "books" }

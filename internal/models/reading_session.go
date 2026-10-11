package models

type ReadingSession struct {
	SessionID     uint64 `json:"session_id" gorm:"primaryKey;column:session_id"`
	UserBookID    uint64 `json:"user_book_id" gorm:"not null;uniqueIndex:ux_sessions_user_book_number"`
	SessionNumber int    `json:"session_number" gorm:"not null;uniqueIndex:ux_sessions_user_book_number"`
	StartDate     Date   `json:"start_date" gorm:"type:date;not null"`
	EndDate       *Date  `json:"end_date" gorm:"type:date"`
	CurrentPage   int    `json:"current_page" gorm:"not null;default:0"`
	Status        string `json:"status" gorm:"size:32;not null;default:READING"`
}

func (ReadingSession) TableName() string { return "reading_sessions" }

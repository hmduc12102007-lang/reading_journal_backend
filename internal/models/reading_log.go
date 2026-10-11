package models

import "time"

type ReadingLog struct {
	LogID     uint64    `json:"log_id" gorm:"primaryKey;column:log_id"`
	SessionID uint64    `json:"session_id" gorm:"not null;index"`
	ReadDate  Date      `json:"read_date" gorm:"type:date;not null;index"`
	StartPage int       `json:"start_page" gorm:"not null"`
	EndPage   int       `json:"end_page" gorm:"not null"`
	Content   *string   `json:"content" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ReadingLog) TableName() string { return "reading_logs" }

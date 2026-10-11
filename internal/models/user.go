package models

import "time"

type User struct {
	UserID       uint64    `json:"user_id" gorm:"primaryKey;column:user_id"`
	Username     string    `json:"username" gorm:"size:100;not null"`
	Email        string    `json:"email" gorm:"size:254;not null;uniqueIndex"`
	PasswordHash string    `json:"-" gorm:"size:255;not null"`
	TokenVersion uint64    `json:"-" gorm:"not null;default:0"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

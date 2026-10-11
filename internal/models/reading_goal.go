package models

import "time"

type ReadingGoal struct {
	GoalID      uint64    `json:"goal_id" gorm:"primaryKey;column:goal_id"`
	UserID      uint64    `json:"user_id" gorm:"not null;index"`
	GoalName    string    `json:"goal_name" gorm:"size:255;not null"`
	GoalType    string    `json:"goal_type" gorm:"size:16;not null"`
	TargetValue int       `json:"target_value" gorm:"not null"`
	StartDate   Date      `json:"start_date" gorm:"type:date;not null"`
	EndDate     Date      `json:"end_date" gorm:"type:date;not null"`
	CreatedAt   time.Time `json:"created_at"`
}

func (ReadingGoal) TableName() string { return "reading_goals" }

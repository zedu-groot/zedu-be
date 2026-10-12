package models

import "time"

type EventRsvp struct {
	EventID          string `gorm:"type:uuid;primaryKey"`
	UserID           string `gorm:"type:uuid;primaryKey"`
	OrganisationID   string `gorm:"type:uuid;not null"`
	Going            bool   `gorm:"not null"`
	ReminderEligible bool   `gorm:"not null"`
	ScheduleVersion  int64  `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

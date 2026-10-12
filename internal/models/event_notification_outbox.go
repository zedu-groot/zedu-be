package models

import "time"

type EventNotificationOutbox struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	EventID         string    `gorm:"type:uuid;not null"`
	OrganisationID  string    `gorm:"type:uuid;not null"`
	UserID          string    `gorm:"type:uuid;not null"`
	Kind            string    `gorm:"size:16;not null"`
	DeliveryKey     string    `gorm:"size:256;not null;uniqueIndex"`
	DueAt           time.Time `gorm:"not null;index:idx_events_due,priority:2"`
	Revision        int64     `gorm:"not null"`
	ScheduleVersion int64     `gorm:"not null"`
	State           string    `gorm:"size:16;not null;index:idx_events_due,priority:1"`
	Attempts        int       `gorm:"not null;default:0"`
	LeaseUntil      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
type EventNotificationJobArgs struct {
	OutboxID string `json:"outbox_id"`
}

func (EventNotificationJobArgs) Kind() string { return "events_notice" }

func (EventNotificationOutbox) TableName() string { return "event_notification_outbox" }

package models

import "time"

type EventHubSettings struct {
	OrganisationID  string  `gorm:"type:uuid;primaryKey"`
	Enabled         bool    `gorm:"not null;default:true"`
	FeaturedEventID *string `gorm:"type:uuid"`
	Revision        int64   `gorm:"not null;default:0"`
	UpdatedAt       time.Time
}

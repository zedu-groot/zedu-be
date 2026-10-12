package models

import "time"

type Event struct {
	ID                string `gorm:"type:uuid;primaryKey"`
	OrganisationID    string `gorm:"type:uuid;not null;uniqueIndex:idx_events_create,priority:1"`
	CreatedBy         string `gorm:"type:uuid;not null;uniqueIndex:idx_events_create,priority:2"`
	OperationID       string `gorm:"type:uuid;not null;uniqueIndex:idx_events_create,priority:3"`
	HostID            string `gorm:"type:uuid;not null"`
	Title             string `gorm:"size:160;not null"`
	Description       string `gorm:"type:text;not null"`
	Category          string `gorm:"size:80;not null"`
	Status            string `gorm:"size:16;not null;index"`
	StartsAt          *time.Time
	EndsAt            *time.Time
	OrganiserTimeZone string `gorm:"size:64;not null"`
	DestinationURL    string `gorm:"size:2048;not null"`
	Revision          int64  `gorm:"not null"`
	ScheduleVersion   int64  `gorm:"not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
type EventDestination struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}
type EventWriteRequest struct {
	OperationID       string            `json:"operation_id" validate:"omitempty,uuid"`
	ExpectedRevision  *int64            `json:"expected_revision" validate:"omitempty,min=1"`
	Title             *string           `json:"title"`
	Description       *string           `json:"description"`
	Category          *string           `json:"category"`
	HostID            *string           `json:"host_id" validate:"omitempty,uuid"`
	OrganiserTimeZone *string           `json:"organiser_time_zone"`
	StartsLocal       *string           `json:"starts_local"`
	EndsLocal         *string           `json:"ends_local"`
	StartsAt          *string           `json:"starts_at"`
	EndsAt            *string           `json:"ends_at"`
	Destination       *EventDestination `json:"destination"`
}
type EventActionRequest struct {
	ExpectedRevision *int64 `json:"expected_revision" validate:"required,min=1"`
}
type EventRsvpRequest struct {
	Going *bool `json:"going" validate:"required"`
}
type EventFeaturedRequest struct {
	EventID          *string `json:"event_id" validate:"omitempty,uuid"`
	ExpectedRevision *int64  `json:"expected_revision" validate:"required,min=0"`
}
type EventCapabilities struct {
	CanManage  bool `json:"can_manage"`
	CanFeature bool `json:"can_feature"`
	CanRsvp    bool `json:"can_rsvp"`
	CanJoin    bool `json:"can_join"`
}
type EventCapabilitiesResponse struct {
	Enabled    bool      `json:"enabled"`
	CanView    bool      `json:"can_view"`
	CanManage  bool      `json:"can_manage"`
	CanFeature bool      `json:"can_feature"`
	ServerNow  time.Time `json:"server_now"`
}
type EventPerson struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RoleLabel string `json:"role_label"`
}
type EventAudience struct {
	OrganisationID string `json:"organisation_id"`
	Label          string `json:"label"`
}
type EventRecord struct {
	ID                string            `json:"id"`
	OrganisationID    string            `json:"organisation_id"`
	Title             string            `json:"title"`
	Description       string            `json:"description"`
	Category          string            `json:"category"`
	Host              EventPerson       `json:"host"`
	Audience          EventAudience     `json:"audience"`
	Status            string            `json:"status"`
	StartsAt          *time.Time        `json:"starts_at"`
	EndsAt            *time.Time        `json:"ends_at"`
	OrganiserTimeZone string            `json:"organiser_time_zone"`
	Revision          int64             `json:"revision"`
	ScheduleVersion   int64             `json:"schedule_version"`
	Featured          bool              `json:"featured"`
	Going             bool              `json:"going"`
	Capabilities      EventCapabilities `json:"capabilities"`
	JoinOpensAt       *time.Time        `json:"join_opens_at"`
	JoinClosesAt      *time.Time        `json:"join_closes_at"`
	ReminderAt        *time.Time        `json:"reminder_at"`
	Destination       *EventDestination `json:"destination,omitempty"`
}
type EventResponse struct {
	Event            EventRecord `json:"event"`
	ServerNow        time.Time   `json:"server_now"`
	FeaturedRevision int64       `json:"featured_revision"`
}
type EventPagination struct {
	CurrentPage     int   `json:"current_page"`
	Limit           int   `json:"limit"`
	TotalItems      int64 `json:"total_items"`
	TotalPagesCount int   `json:"total_pages_count"`
}
type EventListResponse struct {
	Items            []EventRecord   `json:"items"`
	FeaturedEvent    *EventRecord    `json:"featured_event"`
	Pagination       EventPagination `json:"pagination"`
	ServerNow        time.Time       `json:"server_now"`
	FeaturedRevision int64           `json:"featured_revision"`
}
type EventFeatureResponse struct {
	EventID   *string   `json:"event_id"`
	Revision  int64     `json:"revision"`
	ServerNow time.Time `json:"server_now"`
}
type EventJoinResponse struct {
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	ServerNow time.Time `json:"server_now"`
}

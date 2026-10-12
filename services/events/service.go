package events

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage/postgresql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Actor struct {
	OrganisationID string
	UserID         string
}
type EventAccessInput struct {
	Actor
	EventID string
}
type WriteInput struct {
	Actor
	EventID string
	Request models.EventWriteRequest
}
type ActionInput struct {
	Actor
	EventID          string
	ExpectedRevision *int64
}
type RsvpInput struct {
	Actor
	EventID string
	Going   bool
}
type ListInput struct {
	Actor
	Tab, Date, TimeZone, Search string
	Page, Limit                 int
}
type FeaturedInput struct {
	Actor
	EventID          *string
	ExpectedRevision *int64
}
type NoticeInput struct{ OutboxID string }
type Service struct {
	DB            *gorm.DB
	Now           func() time.Time
	PublishNotice func(context.Context, models.OneSignalNotification) error
}
type Error struct {
	Code    int
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Message }
func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return http.StatusInternalServerError
}
func fail(code int, message string) error { return &Error{Code: code, Message: message} }
func field(name, message string) error {
	return &Error{Code: 422, Message: "Please check the event details", Fields: map[string]string{name: message}}
}
func NewService(db *gorm.DB, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{DB: db, Now: clock}
}
func (s *Service) clock() time.Time { return s.Now().UTC() }

type access struct {
	settings     models.EventHubSettings
	permissions  models.PermissionList
	organisation models.Organisation
}

func (s *Service) access(db *gorm.DB, actor Actor) (access, error) {
	var a access
	if _, err := uuid.Parse(actor.OrganisationID); err != nil {
		return a, fail(400, "Invalid organisation")
	}
	if _, err := uuid.Parse(actor.UserID); err != nil {
		return a, fail(401, "Authentication required")
	}
	var member models.OrgUserManagement
	_, err := postgresql.SelectOneFromDb(db.Clauses(clause.Locking{Strength: "SHARE"}), &member, "user_id = ? AND organisation_id = ? AND lower(status) = ? AND is_deactivated = false AND (deleted_at IS NULL OR deleted_at = ?)", actor.UserID, actor.OrganisationID, "active", time.Time{})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return a, fail(403, "Organisation access is unavailable")
	}
	if err != nil {
		return a, err
	}
	var user models.User
	_, err = postgresql.SelectOneFromDb(db.Clauses(clause.Locking{Strength: "SHARE"}), &user, "id = ? AND deactivated = false", actor.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return a, fail(403, "Account access is unavailable")
	}
	if err != nil {
		return a, err
	}
	_, err = postgresql.SelectOneFromDb(db, &a.organisation, "id = ?", actor.OrganisationID)
	if err != nil {
		return a, err
	}
	var permission models.Permission
	_, err = postgresql.SelectOneFromDb(db.Clauses(clause.Locking{Strength: "SHARE"}), &permission, "role_id = ?", member.RoleID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return a, err
	}
	a.permissions = permission.PermissionList
	if a.organisation.OwnerID == actor.UserID {
		a.permissions.CanViewEvents = true
		a.permissions.CanManageEvents = true
		a.permissions.CanFeatureEvents = true
	}
	_, err = postgresql.SelectOneFromDb(db, &a.settings, "organisation_id = ?", actor.OrganisationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		a.settings = models.EventHubSettings{OrganisationID: actor.OrganisationID, Enabled: true, UpdatedAt: s.clock()}
		if _, err = postgresql.CreateIdempotentRecord(db, &a.settings); err != nil {
			return a, err
		}
		_, err = postgresql.SelectOneFromDb(db, &a.settings, "organisation_id = ?", actor.OrganisationID)
	}
	if err != nil {
		return a, err
	}
	return a, nil
}
func (s *Service) authorised(db *gorm.DB, actor Actor) (access, error) {
	a, err := s.access(db, actor)
	if err != nil {
		return a, err
	}
	if !a.settings.Enabled {
		return a, fail(404, "Events Hub is not enabled")
	}
	if !a.permissions.CanViewEvents {
		return a, fail(403, "Events access is unavailable")
	}
	return a, nil
}
func (s *Service) Capabilities(ctx context.Context, actor Actor) (models.EventCapabilitiesResponse, error) {
	a, err := s.access(s.DB.WithContext(ctx), actor)
	r := models.EventCapabilitiesResponse{Enabled: a.settings.Enabled, CanView: a.settings.Enabled && a.permissions.CanViewEvents, CanManage: a.settings.Enabled && a.permissions.CanManageEvents, CanFeature: a.settings.Enabled && a.permissions.CanFeatureEvents, ServerNow: s.clock()}
	return r, err
}
func loadEvent(db *gorm.DB, input EventAccessInput, lock bool) (models.Event, error) {
	var e models.Event
	if _, err := uuid.Parse(input.EventID); err != nil {
		return e, fail(404, "Event not found")
	}
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	_, err := postgresql.SelectOneFromDb(db, &e, "id = ? AND organisation_id = ?", input.EventID, input.OrganisationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return e, fail(404, "Event not found")
	}
	return e, err
}

type projection struct {
	event  models.Event
	actor  Actor
	access access
}

func (s *Service) record(db *gorm.DB, p projection) (models.EventRecord, error) {
	e, actor, a := p.event, p.actor, p.access
	r := models.EventRecord{ID: e.ID, OrganisationID: e.OrganisationID, Title: e.Title, Description: e.Description, Category: e.Category, Status: e.Status, StartsAt: e.StartsAt, EndsAt: e.EndsAt, OrganiserTimeZone: e.OrganiserTimeZone, Revision: e.Revision, ScheduleVersion: e.ScheduleVersion, Audience: models.EventAudience{OrganisationID: e.OrganisationID, Label: a.organisation.Name}, Host: models.EventPerson{ID: e.HostID}, Featured: a.settings.FeaturedEventID != nil && *a.settings.FeaturedEventID == e.ID}
	var user struct{ Name string }
	if err := db.Table("users").Select("name").Where("id = ?", e.HostID).Take(&user).Error; err != nil {
		return r, err
	}
	r.Host.Name = user.Name
	var hostRole struct{ Name string }
	if err := db.Table("org_user_managements m").Select("r.name").Joins("JOIN org_roles r ON r.id::text = m.role_id::text").Where("m.user_id = ? AND m.organisation_id = ?", e.HostID, e.OrganisationID).Scan(&hostRole).Error; err != nil {
		return r, err
	}
	r.Host.RoleLabel = hostRole.Name
	var intent models.EventRsvp
	err := db.Where("event_id = ? AND user_id = ?", e.ID, actor.UserID).First(&intent).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return r, err
	}
	r.Going = intent.Going
	now := s.clock()
	open := e.Status == "published" && e.EndsAt != nil && now.Before(*e.EndsAt)
	r.Capabilities = models.EventCapabilities{CanManage: a.permissions.CanManageEvents, CanFeature: a.permissions.CanFeatureEvents, CanRsvp: open, CanJoin: open && e.StartsAt != nil && JoinAllowed(*e.StartsAt, *e.EndsAt, now) && ValidDestination(e.DestinationURL)}
	if e.StartsAt != nil {
		j := e.StartsAt.Add(-15 * time.Minute)
		r.JoinOpensAt = &j
		r.JoinClosesAt = e.EndsAt
		if intent.Going && intent.ReminderEligible && intent.ScheduleVersion == e.ScheduleVersion {
			n := e.StartsAt.Add(-30 * time.Minute)
			r.ReminderAt = &n
		}
	}
	if a.permissions.CanManageEvents && e.DestinationURL != "" {
		r.Destination = &models.EventDestination{Kind: "external", URL: e.DestinationURL}
	}
	return r, nil
}
func (s *Service) response(db *gorm.DB, p projection) (models.EventResponse, error) {
	r, err := s.record(db, p)
	return models.EventResponse{Event: r, ServerNow: s.clock(), FeaturedRevision: p.access.settings.Revision}, err
}
func (s *Service) Get(ctx context.Context, input EventAccessInput) (models.EventResponse, error) {
	db := s.DB.WithContext(ctx)
	a, err := s.authorised(db, input.Actor)
	if err != nil {
		return models.EventResponse{}, err
	}
	e, err := loadEvent(db, input, false)
	if err != nil {
		return models.EventResponse{}, err
	}
	if e.Status == "draft" && !a.permissions.CanManageEvents {
		return models.EventResponse{}, fail(404, "Event not found")
	}
	return s.response(db, projection{e, input.Actor, a})
}

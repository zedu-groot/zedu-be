package events

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage/postgresql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func optionalTime(local, utc *string) (*time.Time, error) {
	if local == nil && utc == nil {
		return nil, nil
	}
	raw := ""
	if utc != nil {
		raw = *utc
	} else {
		raw = *local
	}
	if raw == "" {
		return nil, nil
	}
	var at time.Time
	var err error
	if utc != nil {
		at, err = time.Parse(time.RFC3339, raw)
	} else {
		zone, _ := time.LoadLocation("Africa/Lagos")
		at, err = time.ParseInLocation("2006-01-02T15:04", raw, zone)
	}
	if err != nil {
		return nil, err
	}
	at = at.UTC()
	return &at, nil
}
func (s *Service) apply(db *gorm.DB, e *models.Event, r models.EventWriteRequest) error {
	if r.Title != nil {
		e.Title = strings.TrimSpace(*r.Title)
	}
	if r.Description != nil {
		e.Description = strings.TrimSpace(*r.Description)
	}
	if r.Category != nil {
		e.Category = strings.TrimSpace(*r.Category)
	}
	for _, f := range []struct {
		name, value string
		max         int
	}{{"title", e.Title, 160}, {"description", e.Description, 5000}, {"category", e.Category, 80}} {
		if utf8.RuneCountInString(f.value) > f.max {
			return field(f.name, "Text is too long")
		}
	}
	if r.OrganiserTimeZone != nil && *r.OrganiserTimeZone != "Africa/Lagos" {
		return field("organiser_time_zone", "Pilot events use Africa/Lagos")
	}
	e.OrganiserTimeZone = "Africa/Lagos"
	if r.HostID != nil {
		e.HostID = *r.HostID
	}
	if _, err := uuid.Parse(e.HostID); err != nil {
		return field("host_id", "Select an organisation host")
	}
	if !postgresql.CheckExists(db, &models.OrgUserManagement{}, "user_id = ? AND organisation_id = ? AND lower(status) = 'active' AND is_deactivated = false AND (deleted_at IS NULL OR deleted_at = ?)", e.HostID, e.OrganisationID, time.Time{}) {
		return field("host_id", "Host must be an active organisation member")
	}
	if !postgresql.CheckExists(db, &models.User{}, "id = ? AND deactivated = false", e.HostID) {
		return field("host_id", "Host account is unavailable")
	}
	if r.StartsLocal != nil || r.StartsAt != nil {
		at, err := optionalTime(r.StartsLocal, r.StartsAt)
		if err != nil {
			return field("starts_local", "Invalid start date/time")
		}
		e.StartsAt = at
	}
	if r.EndsLocal != nil || r.EndsAt != nil {
		at, err := optionalTime(r.EndsLocal, r.EndsAt)
		if err != nil {
			return field("ends_local", "Invalid end date/time")
		}
		e.EndsAt = at
	}
	if e.StartsAt != nil && e.EndsAt != nil && !e.EndsAt.After(*e.StartsAt) {
		return field("ends_local", "End must be after start")
	}
	if r.Destination != nil {
		if r.Destination.Kind != "external" {
			return field("destination", "Use an HTTPS meeting link")
		}
		e.DestinationURL = strings.TrimSpace(r.Destination.URL)
	}
	if e.DestinationURL != "" && !ValidDestination(e.DestinationURL) {
		return field("destination", "Use an HTTPS link without embedded credentials")
	}
	return nil
}
func (s *Service) complete(e models.Event, future bool) error {
	if e.Title == "" {
		return field("title", "A title is required")
	}
	if e.Description == "" {
		return field("description", "A description is required")
	}
	if e.StartsAt == nil {
		return field("starts_local", "A start time is required")
	}
	if e.EndsAt == nil || !e.EndsAt.After(*e.StartsAt) {
		return field("ends_local", "End must be after start")
	}
	if future && !e.StartsAt.After(s.clock()) {
		return field("starts_local", "Publish a future session")
	}
	if !ValidDestination(e.DestinationURL) {
		return field("destination", "An HTTPS joining link is required")
	}
	return nil
}
func (s *Service) Create(ctx context.Context, input WriteInput) (models.EventResponse, error) {
	var response models.EventResponse
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := s.authorised(tx, input.Actor)
		if err != nil {
			return err
		}
		if !a.permissions.CanManageEvents {
			return fail(403, "Event management is unavailable")
		}
		if _, err := uuid.Parse(input.Request.OperationID); err != nil {
			return field("operation_id", "A creation operation UUID is required")
		}
		var old models.Event
		err = tx.Where("organisation_id = ? AND created_by = ? AND operation_id = ?", input.OrganisationID, input.UserID, input.Request.OperationID).First(&old).Error
		if err == nil {
			response, err = s.response(tx, projection{old, input.Actor, a})
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		e := models.Event{ID: uuid.NewString(), OrganisationID: input.OrganisationID, CreatedBy: input.UserID, OperationID: input.Request.OperationID, HostID: input.UserID, Status: "draft", OrganiserTimeZone: "Africa/Lagos", Revision: 1, ScheduleVersion: 1, CreatedAt: s.clock(), UpdatedAt: s.clock()}
		if err := s.apply(tx, &e, input.Request); err != nil {
			return err
		}
		created, err := postgresql.CreateIdempotentRecord(tx, &e)
		if err != nil {
			return err
		}
		if !created {
			e = models.Event{}
			if err := tx.Where("organisation_id = ? AND created_by = ? AND operation_id = ?", input.OrganisationID, input.UserID, input.Request.OperationID).First(&e).Error; err != nil {
				return err
			}
		}
		response, err = s.response(tx, projection{e, input.Actor, a})
		return err
	})
	return response, err
}
func sameStart(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
func (s *Service) Save(ctx context.Context, input WriteInput) (models.EventResponse, error) {
	var response models.EventResponse
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := loadEvent(tx, EventAccessInput{Actor: input.Actor, EventID: input.EventID}, true)
		if err != nil {
			return err
		}
		a, err := s.authorised(tx, input.Actor)
		if err != nil {
			return err
		}
		if !a.permissions.CanManageEvents {
			return fail(403, "Event management is unavailable")
		}
		if e.Status == "cancelled" {
			return fail(422, "Cancelled events cannot be edited")
		}
		old := e
		if err := s.apply(tx, &e, input.Request); err != nil {
			return err
		}
		if input.Request.ExpectedRevision == nil {
			return field("expected_revision", "The event revision is required")
		}
		if reflect.DeepEqual(old, e) {
			response, err = s.response(tx, projection{e, input.Actor, a})
			return err
		}
		if *input.Request.ExpectedRevision != old.Revision {
			return fail(409, "The event changed. Reload and review your edits")
		}
		if old.Status == "published" {
			if old.EndsAt == nil || !s.clock().Before(*old.EndsAt) {
				return fail(422, "Ended events cannot be edited")
			}
			if err := s.complete(e, !sameStart(old.StartsAt, e.StartsAt)); err != nil {
				return err
			}
		}
		e.Revision++
		e.UpdatedAt = s.clock()
		changedStart := !sameStart(old.StartsAt, e.StartsAt)
		if changedStart {
			e.ScheduleVersion++
		}
		if _, err := postgresql.SaveAllFields(tx, &e); err != nil {
			return err
		}
		if old.Status == "published" && (changedStart || !sameStart(old.EndsAt, e.EndsAt) || old.DestinationURL != e.DestinationURL) {
			if err := s.changeNotices(tx, e, changedStart); err != nil {
				return err
			}
		}
		response, err = s.response(tx, projection{e, input.Actor, a})
		return err
	})
	return response, err
}
func (s *Service) action(ctx context.Context, input ActionInput, target string) (models.EventResponse, error) {
	var response models.EventResponse
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := loadEvent(tx, EventAccessInput{Actor: input.Actor, EventID: input.EventID}, true)
		if err != nil {
			return err
		}
		a, err := s.authorised(tx, input.Actor)
		if err != nil {
			return err
		}
		if !a.permissions.CanManageEvents {
			return fail(403, "Event management is unavailable")
		}
		if input.ExpectedRevision == nil {
			return field("expected_revision", "The event revision is required")
		}
		if e.Status == target {
			response, err = s.response(tx, projection{e, input.Actor, a})
			return err
		}
		if *input.ExpectedRevision != e.Revision {
			return fail(409, "The event changed. Reload and try again")
		}
		if target == "published" {
			if e.Status != "draft" {
				return fail(422, "Only drafts can be published")
			}
			if err := s.apply(tx, &e, models.EventWriteRequest{}); err != nil {
				return err
			}
			if err := s.complete(e, true); err != nil {
				return err
			}
		} else if e.Status != "published" {
			return fail(422, "Only published events can be cancelled")
		}
		e.Status = target
		e.Revision++
		e.UpdatedAt = s.clock()
		if _, err := postgresql.SaveAllFields(tx, &e); err != nil {
			return err
		}
		if target == "cancelled" {
			if err := s.suppress(tx, e.ID, ""); err != nil {
				return err
			}
			var recipients []models.EventRsvp
			if err := tx.Where("event_id = ? AND going = true", e.ID).Find(&recipients).Error; err != nil {
				return err
			}
			for _, r := range recipients {
				if err := s.notice(tx, noticeRequest{e, r.UserID, "cancelled", s.clock()}); err != nil {
					return err
				}
			}
			if err := tx.Model(&models.EventHubSettings{}).Where("organisation_id = ? AND featured_event_id = ?", e.OrganisationID, e.ID).Updates(map[string]any{"featured_event_id": nil, "revision": gorm.Expr("revision + 1"), "updated_at": s.clock()}).Error; err != nil {
				return err
			}
			if err := tx.Where("organisation_id = ?", e.OrganisationID).First(&a.settings).Error; err != nil {
				return err
			}
		}
		response, err = s.response(tx, projection{e, input.Actor, a})
		return err
	})
	return response, err
}
func (s *Service) Publish(ctx context.Context, input ActionInput) (models.EventResponse, error) {
	return s.action(ctx, input, "published")
}
func (s *Service) Cancel(ctx context.Context, input ActionInput) (models.EventResponse, error) {
	return s.action(ctx, input, "cancelled")
}
func (s *Service) SetFeatured(ctx context.Context, input FeaturedInput) (models.EventFeatureResponse, error) {
	var result models.EventFeatureResponse
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.EventID != nil {
			e, err := loadEvent(tx, EventAccessInput{Actor: input.Actor, EventID: *input.EventID}, true)
			if err != nil {
				return err
			}
			if e.Status != "published" || e.EndsAt == nil || !s.clock().Before(*e.EndsAt) {
				return fail(422, "Feature a published upcoming event")
			}
		}
		a, err := s.authorised(tx, input.Actor)
		if err != nil {
			return err
		}
		if !a.permissions.CanFeatureEvents {
			return fail(403, "Featured selection is unavailable")
		}
		var settings models.EventHubSettings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organisation_id = ?", input.OrganisationID).First(&settings).Error; err != nil {
			return err
		}
		if input.ExpectedRevision == nil {
			return field("expected_revision", "Featured selection revision is required")
		}
		if !reflect.DeepEqual(settings.FeaturedEventID, input.EventID) {
			if *input.ExpectedRevision != settings.Revision {
				return fail(409, "Featured selection changed. Reload and try again")
			}
			settings.FeaturedEventID = input.EventID
			settings.Revision++
			settings.UpdatedAt = s.clock()
			if _, err := postgresql.SaveAllFields(tx, &settings); err != nil {
				return err
			}
		}
		result = models.EventFeatureResponse{EventID: settings.FeaturedEventID, Revision: settings.Revision, ServerNow: s.clock()}
		return nil
	})
	return result, err
}

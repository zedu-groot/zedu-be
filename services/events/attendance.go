package events

import (
	"context"
	"errors"
	"time"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage/postgresql"
	"gorm.io/gorm"
)

func (s *Service) SetRsvp(ctx context.Context, input RsvpInput) (models.EventResponse, error) {
	var result models.EventResponse
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := loadEvent(tx, EventAccessInput{Actor: input.Actor, EventID: input.EventID}, true)
		if err != nil {
			return err
		}
		a, err := s.authorised(tx, input.Actor)
		if err != nil {
			return err
		}
		if e.Status != "published" || e.EndsAt == nil || !s.clock().Before(*e.EndsAt) {
			return fail(422, "RSVP is closed for this event")
		}
		var r models.EventRsvp
		err = tx.Where("event_id = ? AND user_id = ?", e.ID, input.UserID).First(&r).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		exists := err == nil
		if exists && r.Going == input.Going && r.ScheduleVersion == e.ScheduleVersion {
			result, err = s.response(tx, projection{e, input.Actor, a})
			return err
		}
		if !exists {
			r = models.EventRsvp{EventID: e.ID, UserID: input.UserID, OrganisationID: e.OrganisationID, CreatedAt: s.clock()}
		}
		r.Going = input.Going
		r.ScheduleVersion = e.ScheduleVersion
		r.ReminderEligible = input.Going && e.StartsAt != nil && s.clock().Before(e.StartsAt.Add(-30*time.Minute))
		r.UpdatedAt = s.clock()
		if exists {
			_, err = postgresql.SaveAllFields(tx, &r)
		} else {
			err = postgresql.CreateOneRecord(tx, &r)
		}
		if err != nil {
			return err
		}
		if !r.Going {
			if err := s.suppress(tx, e.ID, input.UserID); err != nil {
				return err
			}
		} else if r.ReminderEligible {
			if err := s.notice(tx, noticeRequest{e, input.UserID, "reminder", e.StartsAt.Add(-30 * time.Minute)}); err != nil {
				return err
			}
		}
		result, err = s.response(tx, projection{e, input.Actor, a})
		return err
	})
	return result, err
}
func (s *Service) ResolveJoin(ctx context.Context, input EventAccessInput) (models.EventJoinResponse, error) {
	var result models.EventJoinResponse
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := loadEvent(tx, input, true)
		if err != nil {
			return err
		}
		if _, err = s.authorised(tx, input.Actor); err != nil {
			return err
		}
		now := s.clock()
		if e.Status != "published" || e.StartsAt == nil || e.EndsAt == nil || !JoinAllowed(*e.StartsAt, *e.EndsAt, now) {
			return fail(403, "Joining is available from fifteen minutes before start until end")
		}
		if !ValidDestination(e.DestinationURL) {
			return field("destination", "The joining link is unavailable. Contact the host")
		}
		result = models.EventJoinResponse{Kind: "external", URL: e.DestinationURL, ServerNow: now}
		return nil
	})
	return result, err
}

package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage/postgresql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type noticeRequest struct {
	event        models.Event
	userID, kind string
	due          time.Time
}

func (s *Service) notice(tx *gorm.DB, input noticeRequest) error {
	e, userID, kind, due := input.event, input.userID, input.kind, input.due
	version := e.Revision
	if kind == "reminder" {
		version = e.ScheduleVersion
	}
	key := fmt.Sprintf("%s:%s:%s:%d", kind, e.ID, userID, version)
	n := models.EventNotificationOutbox{ID: uuid.NewString(), EventID: e.ID, OrganisationID: e.OrganisationID, UserID: userID, Kind: kind, DeliveryKey: key, DueAt: due, Revision: e.Revision, ScheduleVersion: e.ScheduleVersion, State: "pending", CreatedAt: s.clock(), UpdatedAt: s.clock()}
	created, err := postgresql.CreateIdempotentRecord(tx, &n)
	if err != nil {
		return err
	}
	if !created && kind == "reminder" && due.After(s.clock()) {
		return tx.Model(&models.EventNotificationOutbox{}).Where("delivery_key = ? AND state = ?", key, "suppressed").Updates(map[string]any{"state": "pending", "lease_until": nil, "updated_at": s.clock()}).Error
	}
	return nil
}
func (s *Service) suppress(tx *gorm.DB, eventID, userID string) error {
	q := tx.Model(&models.EventNotificationOutbox{}).Where("event_id = ? AND kind = ? AND state = ?", eventID, "reminder", "pending")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	return q.Updates(map[string]any{"state": "suppressed", "lease_until": nil, "updated_at": s.clock()}).Error
}
func (s *Service) changeNotices(tx *gorm.DB, e models.Event, changedStart bool) error {
	var recipients []models.EventRsvp
	if err := tx.Where("event_id = ? AND going = true", e.ID).Find(&recipients).Error; err != nil {
		return err
	}
	if changedStart {
		if err := s.suppress(tx, e.ID, ""); err != nil {
			return err
		}
	}
	for _, r := range recipients {
		if changedStart {
			r.ScheduleVersion = e.ScheduleVersion
			r.ReminderEligible = e.StartsAt != nil && s.clock().Before(e.StartsAt.Add(-30*time.Minute))
			r.UpdatedAt = s.clock()
			if _, err := postgresql.SaveAllFields(tx, &r); err != nil {
				return err
			}
			if r.ReminderEligible {
				if err := s.notice(tx, noticeRequest{e, r.UserID, "reminder", e.StartsAt.Add(-30 * time.Minute)}); err != nil {
					return err
				}
			}
		}
		if err := s.notice(tx, noticeRequest{e, r.UserID, "updated", s.clock()}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) DeliverNotice(ctx context.Context, input NoticeInput) error {
	var snapshot models.EventNotificationOutbox
	if err := s.DB.WithContext(ctx).Where("id = ?", input.OutboxID).First(&snapshot).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	var emitted *models.OneSignalNotification
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := loadEvent(tx, EventAccessInput{Actor: Actor{OrganisationID: snapshot.OrganisationID, UserID: snapshot.UserID}, EventID: snapshot.EventID}, true)
		if err != nil {
			return err
		}
		var n models.EventNotificationOutbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", snapshot.ID).First(&n).Error; err != nil {
			return err
		}
		if n.State != "pending" || s.clock().Before(n.DueAt) {
			return nil
		}
		a, accessErr := s.authorised(tx, Actor{OrganisationID: n.OrganisationID, UserID: n.UserID})
		_ = a
		allowed := accessErr == nil
		if accessErr != nil && Status(accessErr) >= 500 {
			return accessErr
		}
		var r models.EventRsvp
		err = tx.Where("event_id = ? AND user_id = ?", e.ID, n.UserID).First(&r).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		allowed = allowed && r.Going
		switch n.Kind {
		case "reminder":
			allowed = allowed && e.Status == "published" && e.StartsAt != nil && s.clock().Before(*e.StartsAt) && n.ScheduleVersion == e.ScheduleVersion && r.ReminderEligible && r.ScheduleVersion == e.ScheduleVersion
		case "updated":
			allowed = allowed && e.Status == "published" && n.Revision <= e.Revision
		case "cancelled":
			allowed = allowed && e.Status == "cancelled"
		default:
			allowed = false
		}
		state := "suppressed"
		if allowed {
			message := "The event was cancelled."
			title := "Event cancelled"
			if n.Kind == "reminder" {
				title = "Event reminder"
				zone, _ := time.LoadLocation(e.OrganiserTimeZone)
				message = fmt.Sprintf("%s starts at %s WAT.", e.Title, e.StartsAt.In(zone).Format("Mon 2 Jan, 3:04 PM"))
			} else if n.Kind == "updated" {
				title = "Event updated"
				message = "The schedule or joining location changed. Open the event for current details."
			}
			org := e.OrganisationID
			inbox := models.OneSignalNotification{ID: n.ID, OneSignalNotificationID: "event:" + n.ID, UserID: n.UserID, OrgID: &org, Title: title, Message: message, Status: models.OneSignalNotificationStatusPending, SentAt: s.clock(), CreatedAt: s.clock(), UpdatedAt: s.clock(), Payload: map[string]any{"notification_type": "event_" + n.Kind, "event_id": e.ID, "org_id": e.OrganisationID, "notification_id": n.ID, "revision": e.Revision, "delivery_channel": "in_app"}}
			created, err := postgresql.CreateIdempotentRecord(tx, &inbox)
			if err != nil {
				return err
			}
			if created {
				emitted = &inbox
			}
			state = "delivered"
		}
		return tx.Model(&n).Updates(map[string]any{"state": state, "lease_until": nil, "updated_at": s.clock()}).Error
	})
	if err != nil {
		return err
	}
	if emitted != nil && s.PublishNotice != nil {
		s.PublishNotice(ctx, *emitted)
	}
	return nil
}

type ReconcileInput struct {
	Enqueue func(context.Context, string) error
	Limit   int
}

func (s *Service) ReconcileDueNotices(ctx context.Context, input ReconcileInput) error {
	if input.Limit <= 0 || input.Limit > 100 {
		input.Limit = 100
	}
	var claimed []models.EventNotificationOutbox
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("state = ? AND due_at <= ? AND (lease_until IS NULL OR lease_until <= ?)", "pending", s.clock(), s.clock()).Order("due_at ASC").Limit(input.Limit).Find(&claimed).Error; err != nil {
			return err
		}
		lease := s.clock().Add(5 * time.Minute)
		for _, n := range claimed {
			if err := tx.Model(&n).Updates(map[string]any{"lease_until": lease, "attempts": gorm.Expr("attempts + 1"), "updated_at": s.clock()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	var firstErr error
	for _, n := range claimed {
		var workErr error
		if input.Enqueue != nil {
			workErr = input.Enqueue(ctx, n.ID)
		} else {
			workErr = s.DeliverNotice(ctx, NoticeInput{OutboxID: n.ID})
		}
		if workErr != nil {
			if err := s.DB.WithContext(ctx).Model(&models.EventNotificationOutbox{}).Where("id = ? AND state = ?", n.ID, "pending").Update("lease_until", nil).Error; err != nil {
				return err
			}
			if firstErr == nil {
				firstErr = workErr
			}
		}
	}
	return firstErr
}

package events

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/hngprojects/telex_be/internal/models"
	"gorm.io/gorm"
)

func (s *Service) List(ctx context.Context, input ListInput) (models.EventListResponse, error) {
	r := models.EventListResponse{Items: []models.EventRecord{}, ServerNow: s.clock()}
	db := s.DB.WithContext(ctx)
	a, err := s.authorised(db, input.Actor)
	if err != nil {
		return r, err
	}
	r.FeaturedRevision = a.settings.Revision
	if input.Tab == "" {
		input.Tab = "upcoming"
	}
	if input.Page < 1 {
		input.Page = 1
	}
	if input.Limit < 1 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}
	if input.Page > 1000000 {
		return r, field("page", "Page is too large")
	}
	if len(input.Search) > 200 {
		return r, field("q", "Search is too long")
	}
	q := db.Model(&models.Event{}).Where("organisation_id = ?", input.OrganisationID)
	order := "starts_at ASC NULLS LAST, id ASC"
	switch input.Tab {
	case "upcoming":
		q = q.Where("status = ? AND ends_at > ?", "published", s.clock())
	case "past":
		q = q.Where("(status = ? OR (status = ? AND ends_at <= ?))", "cancelled", "published", s.clock())
		order = "starts_at DESC NULLS LAST, id ASC"
	case "my-rsvps":
		q = q.Where("status <> ? AND id IN (SELECT event_id FROM event_rsvps WHERE user_id = ? AND going = true)", "draft", input.UserID)
	case "drafts":
		if !a.permissions.CanManageEvents {
			return r, fail(403, "Draft management is unavailable")
		}
		q = q.Where("status = ?", "draft")
		order = "updated_at DESC, id ASC"
	default:
		return r, field("tab", "Unknown events tab")
	}
	if input.Date != "" {
		zoneName := input.TimeZone
		if zoneName == "" {
			zoneName = "Africa/Lagos"
		}
		zone, e := time.LoadLocation(zoneName)
		if e != nil {
			return r, field("time_zone", "Invalid time zone")
		}
		day, e := time.ParseInLocation("2006-01-02", input.Date, zone)
		if e != nil {
			return r, field("date", "Invalid date")
		}
		q = q.Where("starts_at >= ? AND starts_at < ?", day.UTC(), day.AddDate(0, 0, 1).UTC())
	}
	if input.Search != "" {
		term := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(input.Search, "\\", "\\\\"), "%", "\\%"), "_", "\\_")
		q = q.Where("(title ILIKE ? OR description ILIKE ?)", "%"+term+"%", "%"+term+"%")
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return r, err
	}
	var rows []models.Event
	if err := q.Session(&gorm.Session{}).Order(order).Offset((input.Page - 1) * input.Limit).Limit(input.Limit).Find(&rows).Error; err != nil {
		return r, err
	}
	for _, e := range rows {
		item, err := s.record(db, projection{e, input.Actor, a})
		if err != nil {
			return r, err
		}
		r.Items = append(r.Items, item)
	}
	if a.settings.FeaturedEventID != nil && input.Tab != "drafts" {
		var featured models.Event
		if err := q.Session(&gorm.Session{}).Where("id = ? AND status = ? AND ends_at > ?", *a.settings.FeaturedEventID, "published", s.clock()).First(&featured).Error; err == nil {
			item, err := s.record(db, projection{featured, input.Actor, a})
			if err != nil {
				return r, err
			}
			r.FeaturedEvent = &item
		}
	}
	r.Pagination = models.EventPagination{CurrentPage: input.Page, Limit: input.Limit, TotalItems: total, TotalPagesCount: int(math.Ceil(float64(total) / float64(input.Limit)))}
	return r, nil
}

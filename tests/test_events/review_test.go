package test_events

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
	events "github.com/hngprojects/telex_be/services/events"
	"gorm.io/gorm"
)

func TestConcurrentCreateRetries(t *testing.T) {
	f := setup(t)
	enable(t, f)
	operation := uuid.NewString()
	var barrier sync.WaitGroup
	barrier.Add(2)
	f.db.Callback().Create().Before("gorm:create").Register("review_create_barrier", func(tx *gorm.DB) {
		event, ok := tx.Statement.Dest.(*models.Event)
		if ok && event.OperationID == operation {
			barrier.Done()
			barrier.Wait()
		}
	})
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := f.s.Create(context.Background(), events.WriteInput{Actor: f.host, Request: models.EventWriteRequest{OperationID: operation, Title: str("Concurrent draft")}})
			errs <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("same operation create returned status %d: %v", events.Status(err), err)
		}
	}
}

func TestExplicitFalseEventPermissionSurvivesSerialization(t *testing.T) {
	value, err := (models.PermissionList{CanManageChannels: true}).Value()
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(value.([]byte), &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"can_view_events", "can_manage_events", "can_feature_events"} {
		if v, exists := fields[key]; !exists || v != false {
			t.Errorf("explicit denial lost for %s: %v", key, fields)
		}
	}
}
func TestTextEditPreservesPendingScheduleNotice(t *testing.T) {
	f := setup(t)
	enable(t, f)
	ctx := context.Background()
	e := published(t, f)
	if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true}); err != nil {
		t.Fatal(err)
	}
	changed, err := f.s.Save(ctx, events.WriteInput{Actor: f.host, EventID: e.Event.ID, Request: models.EventWriteRequest{ExpectedRevision: rev(e.Event.Revision), StartsLocal: str("2026-10-16T17:00"), EndsLocal: str("2026-10-16T18:00")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Save(ctx, events.WriteInput{Actor: f.host, EventID: e.Event.ID, Request: models.EventWriteRequest{ExpectedRevision: rev(changed.Event.Revision), Title: str("Updated title")}}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ReconcileDueNotices(ctx, events.ReconcileInput{}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := f.db.Model(&models.OneSignalNotification{}).Where("title = ?", "Event updated").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("schedule update notice lost after text edit: %d", count)
	}
}

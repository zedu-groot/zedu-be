package test_events

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
	events "github.com/hngprojects/telex_be/services/events"
)

func TestCancellationReturnsCurrentFeaturedRevision(t *testing.T) {
	f := setup(t)
	enable(t, f)
	ctx := context.Background()
	e := published(t, f)
	selected, err := f.s.SetFeatured(ctx, events.FeaturedInput{Actor: f.lead, EventID: &e.Event.ID, ExpectedRevision: rev(0)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.s.Cancel(ctx, events.ActionInput{Actor: f.host, EventID: e.Event.ID, ExpectedRevision: rev(e.Event.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	if result.FeaturedRevision != selected.Revision+1 {
		t.Fatalf("cancellation returned stale featured revision: %d", result.FeaturedRevision)
	}
}
func TestEnqueueFailureDoesNotDelayOtherRecipients(t *testing.T) {
	f := setup(t)
	enable(t, f)
	ctx := context.Background()
	e := published(t, f)
	for _, actor := range []events.Actor{f.member, f.lead} {
		if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: actor, EventID: e.Event.ID, Going: true}); err != nil {
			t.Fatal(err)
		}
	}
	f.now = time.Date(2026, 10, 16, 14, 30, 0, 0, time.UTC)
	calls := 0
	err := f.s.ReconcileDueNotices(ctx, events.ReconcileInput{Enqueue: func(ctx context.Context, id string) error {
		calls++
		if calls == 1 {
			return errors.New("queue unavailable")
		}
		return f.s.DeliverNotice(ctx, events.NoticeInput{OutboxID: id})
	}})
	if err == nil {
		t.Fatal("queue failure was concealed")
	}
	var count int64
	f.db.Model(&models.OneSignalNotification{}).Count(&count)
	if count != 1 {
		t.Fatalf("unaffected recipient was delayed: %d notices", count)
	}
	if err := f.s.ReconcileDueNotices(ctx, events.ReconcileInput{}); err != nil {
		t.Fatal(err)
	}
	f.db.Model(&models.OneSignalNotification{}).Count(&count)
	if count != 2 {
		t.Fatalf("recovery lost notice: %d", count)
	}
}
func TestReschedulingAndDuplicateWorkers(t *testing.T) {
	f := setup(t)
	enable(t, f)
	ctx := context.Background()
	e := published(t, f)
	if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true}); err != nil {
		t.Fatal(err)
	}
	var old models.EventNotificationOutbox
	f.db.Where("kind = ?", "reminder").First(&old)
	edited, err := f.s.Save(ctx, events.WriteInput{Actor: f.host, EventID: e.Event.ID, Request: models.EventWriteRequest{ExpectedRevision: rev(e.Event.Revision), StartsLocal: str("2026-10-16T17:00"), EndsLocal: str("2026-10-16T18:00")}})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Event.ScheduleVersion != e.Event.ScheduleVersion+1 {
		t.Fatal("schedule identity not advanced")
	}
	f.now = time.Date(2026, 10, 16, 14, 30, 0, 0, time.UTC)
	if err := f.s.DeliverNotice(ctx, events.NoticeInput{OutboxID: old.ID}); err != nil {
		t.Fatal(err)
	}
	var count int64
	f.db.Model(&models.OneSignalNotification{}).Count(&count)
	if count != 0 {
		t.Fatal("obsolete reminder delivered")
	}
	var current models.EventNotificationOutbox
	f.db.Where("kind = ? AND state = ?", "reminder", "pending").First(&current)
	f.now = time.Date(2026, 10, 16, 15, 30, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.s.DeliverNotice(ctx, events.NoticeInput{OutboxID: current.ID}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	f.db.Model(&models.OneSignalNotification{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate worker inbox records: %d", count)
	}
}
func TestRevokedRoleAndCancelledRsvpSuppressReminder(t *testing.T) {
	f := setup(t)
	enable(t, f)
	ctx := context.Background()
	e := published(t, f)
	if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true}); err != nil {
		t.Fatal(err)
	}
	f.db.Exec("UPDATE permissions SET permission_list = '{}'::jsonb WHERE role_id = (SELECT role_id FROM org_user_managements WHERE user_id = ? AND organisation_id = ?)", f.member.UserID, f.member.OrganisationID)
	if _, err := f.s.Get(ctx, events.EventAccessInput{Actor: f.member, EventID: e.Event.ID}); events.Status(err) != 403 {
		t.Fatalf("stale role access: %v", err)
	}
	f.now = time.Date(2026, 10, 16, 14, 30, 0, 0, time.UTC)
	if err := f.s.ReconcileDueNotices(ctx, events.ReconcileInput{}); err != nil {
		t.Fatal(err)
	}
	var count int64
	f.db.Model(&models.OneSignalNotification{}).Count(&count)
	if count != 0 {
		t.Fatal("removed event permission received reminder")
	}
}
func TestPermissionBackfillPreservesExplicitAndCustomGrants(t *testing.T) {
	f := setup(t)
	roles := []struct {
		name        string
		defaultRole bool
		value       string
		wantView    bool
	}{{"User", true, `{"can_manage_channels":true}`, true}, {"User", true, `{"can_view_events":false}`, false}, {"Custom", false, `{"can_manage_channels":true}`, false}}
	for _, r := range roles {
		role := uuid.NewString()
		id := uuid.NewString()
		f.db.Exec("INSERT INTO org_roles(id,name,is_default) VALUES (?,?,?)", role, r.name, r.defaultRole)
		f.db.Exec("INSERT INTO permissions(id,role_id,permission_list) VALUES (?,?,?::jsonb)", id, role, r.value)
		sql, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000003_events_hub.up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
		var p models.Permission
		if err := f.db.Where("id = ?", id).First(&p).Error; err != nil {
			t.Fatal(err)
		}
		if p.PermissionList.CanViewEvents != r.wantView {
			t.Fatalf("backfill %s: %v", r.name, p.PermissionList)
		}
	}
}

package test_events

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
	events "github.com/hngprojects/telex_be/services/events"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixture struct {
	db                        *gorm.DB
	s                         *events.Service
	host, member, lead, other events.Actor
	now                       time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("EVENTS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("EVENTS_TEST_DATABASE_URL is required for disposable PostgreSQL integration tests")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schema := "events_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		a, _ := admin.DB()
		a.Close()
	})
	statements := []string{
		"CREATE TABLE users (id uuid PRIMARY KEY,name text,deactivated boolean DEFAULT false,is_active boolean DEFAULT true,deleted_at timestamptz)",
		"CREATE TABLE organisations (id uuid PRIMARY KEY,name text,owner_id uuid,deleted_at timestamptz)",
		"CREATE TABLE org_roles (id uuid PRIMARY KEY,name text,is_default boolean DEFAULT false)",
		"CREATE TABLE permissions (id uuid PRIMARY KEY,role_id uuid,permission_list jsonb,deleted_at timestamptz)",
		"CREATE TABLE org_user_managements (user_id uuid,organisation_id uuid,status text,is_deactivated boolean DEFAULT false,role_id uuid,deleted_at timestamptz,PRIMARY KEY(user_id,organisation_id))",
		"CREATE TABLE onesignal_notifications (id uuid PRIMARY KEY,onesignal_notification_id text NOT NULL,user_id text NOT NULL,org_id uuid,title text,message text,payload jsonb,avatar_url text,status text,sent_at timestamptz,delivered_at timestamptz,read_at timestamptz,created_at timestamptz,updated_at timestamptz,deleted_at timestamptz)",
	}
	for _, sql := range statements {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	sql, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000003_events_hub.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(sql)).Error; err != nil {
		t.Fatal(err)
	}
	org := uuid.NewString()
	host := uuid.NewString()
	member := uuid.NewString()
	lead := uuid.NewString()
	other := uuid.NewString()
	if err := db.Exec("INSERT INTO organisations(id,name,owner_id) VALUES (?, ?, ?)", org, "Test cohort", host).Error; err != nil {
		t.Fatal(err)
	}
	for _, person := range []struct {
		id, role string
		perms    map[string]bool
	}{{host, "Owner", map[string]bool{"can_view_events": true, "can_manage_events": true, "can_feature_events": true}}, {member, "User", map[string]bool{"can_view_events": true}}, {lead, "Project Lead", map[string]bool{"can_view_events": true, "can_feature_events": true}}, {other, "Other", nil}} {
		role := uuid.NewString()
		db.Exec("INSERT INTO users(id,name) VALUES (?,?)", person.id, person.role)
		if person.perms == nil {
			continue
		}
		p, _ := json.Marshal(person.perms)
		db.Exec("INSERT INTO org_roles(id,name) VALUES (?,?)", role, person.role)
		db.Exec("INSERT INTO permissions(id,role_id,permission_list) VALUES (?,?,?::jsonb)", uuid.NewString(), role, string(p))
		db.Exec("INSERT INTO org_user_managements(user_id,organisation_id,role_id,status) VALUES (?,?,?,?)", person.id, org, role, "Active")
	}
	f := &fixture{db: db, now: time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC), host: events.Actor{OrganisationID: org, UserID: host}, member: events.Actor{OrganisationID: org, UserID: member}, lead: events.Actor{OrganisationID: org, UserID: lead}, other: events.Actor{OrganisationID: org, UserID: other}}
	f.s = events.NewService(db, func() time.Time { return f.now })
	return f
}
func enable(t *testing.T, f *fixture) {
	t.Helper()
	if err := f.db.Create(&models.EventHubSettings{OrganisationID: f.host.OrganisationID, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
}
func str(s string) *string { return &s }
func rev(n int64) *int64   { return &n }
func published(t *testing.T, f *fixture) models.EventResponse {
	t.Helper()
	r, err := f.s.Create(context.Background(), events.WriteInput{Actor: f.host, Request: models.EventWriteRequest{OperationID: uuid.NewString(), Title: str("Workshop"), Description: str("A shared learning session"), Category: str("Workshop"), StartsLocal: str("2026-10-16T16:00"), EndsLocal: str("2026-10-16T17:00"), Destination: &models.EventDestination{Kind: "external", URL: "https://meet.example.test/session"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err = f.s.Publish(context.Background(), events.ActionInput{Actor: f.host, EventID: r.Event.ID, ExpectedRevision: rev(r.Event.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestDefaultOnExplicitDisableAndFreshMembershipChecks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	caps, err := f.s.Capabilities(ctx, f.member)
	if err != nil || !caps.Enabled || !caps.CanView {
		t.Fatalf("Events must be available without activation: %v %v", caps, err)
	}
	e := published(t, f)
	if _, err := f.s.SetFeatured(ctx, events.FeaturedInput{Actor: f.lead, EventID: &e.Event.ID, ExpectedRevision: rev(0)}); err != nil {
		t.Fatalf("default settings must support featuring: %v", err)
	}
	if err := f.db.Model(&models.EventHubSettings{}).Where("organisation_id = ?", f.host.OrganisationID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	caps, err = f.s.Capabilities(ctx, f.member)
	if err != nil || caps.Enabled {
		t.Fatalf("explicit disable ignored: %v %v", caps, err)
	}
	if _, err := f.s.List(ctx, events.ListInput{Actor: f.member}); events.Status(err) != 404 {
		t.Fatalf("disabled hub still available: %v", err)
	}
	if _, err := f.s.Capabilities(ctx, f.other); events.Status(err) != 403 {
		t.Fatalf("nonmember: %v", err)
	}
	f.db.Exec("UPDATE org_user_managements SET is_deactivated=true WHERE user_id=?", f.member.UserID)
	if _, err := f.s.Capabilities(ctx, f.member); events.Status(err) != 403 {
		t.Fatalf("revoked member: %v", err)
	}
}
func TestDraftPublishRetryConflictAndSharedDiscovery(t *testing.T) {
	f := setup(t)
	enable(t, f)
	ctx := context.Background()
	op := uuid.NewString()
	input := events.WriteInput{Actor: f.host, Request: models.EventWriteRequest{OperationID: op, Title: str("Draft")}}
	draft, err := f.s.Create(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.Create(ctx, input)
	if err != nil || retry.Event.ID != draft.Event.ID {
		t.Fatalf("duplicate create: %v", err)
	}
	if _, err := f.s.Get(ctx, events.EventAccessInput{Actor: f.member, EventID: draft.Event.ID}); events.Status(err) != 404 {
		t.Fatalf("draft leaked to member: %v", err)
	}
	if _, err := f.s.Publish(ctx, events.ActionInput{Actor: f.host, EventID: draft.Event.ID, ExpectedRevision: rev(draft.Event.Revision)}); events.Status(err) != 422 {
		t.Fatalf("incomplete publication: %v", err)
	}
	e := published(t, f)
	list, err := f.s.List(ctx, events.ListInput{Actor: f.member, Tab: "upcoming", Page: 1, Limit: 20})
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != e.Event.ID {
		t.Fatalf("shared discovery: %v %v", list, err)
	}
	_, err = f.s.Save(ctx, events.WriteInput{Actor: f.host, EventID: e.Event.ID, Request: models.EventWriteRequest{ExpectedRevision: rev(e.Event.Revision - 1), Title: str("Overwrite")}})
	if events.Status(err) != 409 {
		t.Fatalf("stale edit: %v", err)
	}
	if list.Items[0].Destination != nil {
		t.Fatal("member list exposed joining destination")
	}
}
func TestRsvpConcurrencyAndServerJoinChecks(t *testing.T) {
	f := setup(t)
	enable(t, f)
	e := published(t, f)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var count int64
	f.db.Model(&models.EventRsvp{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate RSVP: %d", count)
	}
	f.db.Model(&models.EventNotificationOutbox{}).Where("kind = ?", "reminder").Count(&count)
	if count != 1 {
		t.Fatalf("duplicate reminders: %d", count)
	}
	f.now = time.Date(2026, 10, 16, 14, 44, 59, 0, time.UTC)
	if _, err := f.s.ResolveJoin(ctx, events.EventAccessInput{Actor: f.member, EventID: e.Event.ID}); events.Status(err) != 403 {
		t.Fatalf("early join: %v", err)
	}
	f.now = f.now.Add(time.Second)
	join, err := f.s.ResolveJoin(ctx, events.EventAccessInput{Actor: f.member, EventID: e.Event.ID})
	if err != nil || join.URL != "https://meet.example.test/session" {
		t.Fatalf("join: %v %v", join, err)
	}
	f.db.Exec("DELETE FROM org_user_managements WHERE user_id=?", f.member.UserID)
	if _, err := f.s.ResolveJoin(ctx, events.EventAccessInput{Actor: f.member, EventID: e.Event.ID}); events.Status(err) != 403 {
		t.Fatalf("revoked join: %v", err)
	}
}
func TestDurableReminderRetryAndCancellation(t *testing.T) {
	f := setup(t)
	enable(t, f)
	e := published(t, f)
	ctx := context.Background()
	_, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true})
	if err != nil {
		t.Fatal(err)
	}
	f.now = time.Date(2026, 10, 16, 14, 30, 0, 0, time.UTC)
	var job models.EventNotificationOutbox
	if err := f.db.First(&job).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := f.s.DeliverNotice(ctx, events.NoticeInput{OutboxID: job.ID}); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	f.db.Model(&models.OneSignalNotification{}).Count(&count)
	if count != 1 {
		t.Fatalf("inbox duplicates: %d", count)
	}
	_, err = f.s.Cancel(ctx, events.ActionInput{Actor: f.host, EventID: e.Event.ID, ExpectedRevision: rev(e.Event.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ResolveJoin(ctx, events.EventAccessInput{Actor: f.member, EventID: e.Event.ID}); events.Status(err) != 403 {
		t.Fatalf("cancelled join: %v", err)
	}
	if _, err := f.s.SetRsvp(ctx, events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: false}); events.Status(err) != 422 {
		t.Fatalf("cancelled RSVP controls: %v", err)
	}
}
func TestLateRsvpDoesNotGetPastReminderAndSearchAcrossPages(t *testing.T) {
	f := setup(t)
	enable(t, f)
	e := published(t, f)
	f.now = time.Date(2026, 10, 16, 14, 30, 0, 0, time.UTC)
	if _, err := f.s.SetRsvp(context.Background(), events.RsvpInput{Actor: f.member, EventID: e.Event.ID, Going: true}); err != nil {
		t.Fatal(err)
	}
	var count int64
	f.db.Model(&models.EventNotificationOutbox{}).Where("kind = ?", "reminder").Count(&count)
	if count != 0 {
		t.Fatal("late RSVP queued past reminder")
	}
	result, err := f.s.List(context.Background(), events.ListInput{Actor: f.member, Tab: "upcoming", Search: "workshop", Page: 1, Limit: 1})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("search: %v %v", result, err)
	}
}

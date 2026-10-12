package test_events

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/models"
)

func TestMigrationAfterAutoMigrateInstallsConstraints(t *testing.T) {
	f := setup(t)
	if err := f.db.Exec("DROP TABLE event_notification_outbox, event_hub_settings, event_rsvps, events CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.AutoMigrate(&models.Event{}, &models.EventRsvp{}, &models.EventHubSettings{}, &models.EventNotificationOutbox{}); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000003_events_hub.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := f.db.Raw("SELECT count(*) FROM pg_constraint WHERE connamespace = current_schema()::regnamespace AND conrelid IN ('events'::regclass,'event_rsvps'::regclass,'event_hub_settings'::regclass,'event_notification_outbox'::regclass) AND contype IN ('f','c','u')").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	// GORM's unique indexes do not replace referential and publication checks.
	if count < 15 {
		t.Fatalf("missing integrity constraints after GORM-first migration: got %d, want at least 15", count)
	}
	var enabled bool
	if err := f.db.Exec("INSERT INTO event_hub_settings(organisation_id) VALUES (?)", f.host.OrganisationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Table("event_hub_settings").Select("enabled").Where("organisation_id = ?", f.host.OrganisationID).Scan(&enabled).Error; err != nil || !enabled {
		t.Fatalf("migration default must be ON: %v %v", enabled, err)
	}

	e := published(t, f)
	if err := f.db.Exec("UPDATE events SET status = 'invalid' WHERE id = ?", e.Event.ID).Error; err == nil {
		t.Fatal("invalid event status accepted")
	}
	if err := f.db.Exec("UPDATE events SET ends_at = starts_at WHERE id = ?", e.Event.ID).Error; err == nil {
		t.Fatal("invalid event schedule accepted")
	}
	if err := f.db.Exec("UPDATE events SET title = '' WHERE id = ?", e.Event.ID).Error; err == nil {
		t.Fatal("incomplete publication accepted")
	}
	if err := f.db.Exec("INSERT INTO event_rsvps(event_id,user_id,organisation_id,going,reminder_eligible,schedule_version) VALUES (?,?,?,false,false,1)", e.Event.ID, f.member.UserID, uuid.NewString()).Error; err == nil {
		t.Fatal("cross-organisation RSVP accepted")
	}
	if err := f.db.Exec("UPDATE event_hub_settings SET featured_event_id = ?", uuid.NewString()).Error; err == nil {
		t.Fatal("missing featured event accepted")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000003_events_hub.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(string(down)).Error; err != nil {
		t.Fatal(err)
	}
	var users int64
	if err := f.db.Table("users").Count(&users).Error; err != nil || users != 4 {
		t.Fatalf("rollback damaged unrelated users: count %d error %v", users, err)
	}
}

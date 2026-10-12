DROP TABLE IF EXISTS event_notification_outbox;
DROP TABLE IF EXISTS event_hub_settings;
DROP TABLE IF EXISTS event_rsvps;
DROP TABLE IF EXISTS events;
-- Preserve role permission JSON; rolling back feature tables must not rewrite unrelated grants.

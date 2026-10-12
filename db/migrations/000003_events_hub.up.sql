CREATE TABLE IF NOT EXISTS events (
 id uuid PRIMARY KEY, organisation_id uuid NOT NULL, created_by uuid NOT NULL, operation_id uuid NOT NULL,
 host_id uuid NOT NULL, title varchar(160) NOT NULL DEFAULT '', description text NOT NULL DEFAULT '', category varchar(80) NOT NULL DEFAULT '',
 status varchar(16) NOT NULL DEFAULT 'draft', starts_at timestamptz, ends_at timestamptz,
 organiser_time_zone varchar(64) NOT NULL DEFAULT 'Africa/Lagos', destination_url varchar(2048) NOT NULL DEFAULT '', revision bigint NOT NULL DEFAULT 1, schedule_version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_events_org_schedule ON events(organisation_id,status,starts_at,id);
CREATE TABLE IF NOT EXISTS event_rsvps (
 event_id uuid NOT NULL,user_id uuid NOT NULL,organisation_id uuid NOT NULL,going boolean NOT NULL DEFAULT false,
 reminder_eligible boolean NOT NULL DEFAULT false,schedule_version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,user_id)
);
CREATE INDEX IF NOT EXISTS idx_event_rsvps_member ON event_rsvps(organisation_id,user_id,going);
CREATE TABLE IF NOT EXISTS event_hub_settings (
 organisation_id uuid PRIMARY KEY,enabled boolean NOT NULL DEFAULT false,featured_event_id uuid,revision bigint NOT NULL DEFAULT 0,updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS event_notification_outbox (
 id uuid PRIMARY KEY,event_id uuid NOT NULL,organisation_id uuid NOT NULL,user_id uuid NOT NULL,kind varchar(16) NOT NULL,
 delivery_key varchar(256) NOT NULL UNIQUE,due_at timestamptz NOT NULL,revision bigint NOT NULL,schedule_version bigint NOT NULL,state varchar(16) NOT NULL DEFAULT 'pending',
 attempts integer NOT NULL DEFAULT 0,lease_until timestamptz,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_events_due ON event_notification_outbox(state,due_at);
-- AutoMigrate may create these tables first. Install invariants independently of
-- CREATE TABLE so both deployment orders converge on the same schema.
DO $$
DECLARE item record;
BEGIN
 FOR item IN SELECT * FROM (VALUES
  ('events', 'events_org_identity', 'UNIQUE(id,organisation_id)'),
  ('events', 'events_creation_operation', 'UNIQUE(organisation_id,created_by,operation_id)'),
  ('events', 'events_organisation_fk', 'FOREIGN KEY(organisation_id) REFERENCES organisations(id)'),
  ('events', 'events_creator_fk', 'FOREIGN KEY(created_by) REFERENCES users(id)'),
  ('events', 'events_host_fk', 'FOREIGN KEY(host_id) REFERENCES users(id)'),
  ('events', 'events_status_valid', 'CHECK(status IN (''draft'',''published'',''cancelled''))'),
  ('events', 'events_schedule_order', 'CHECK(starts_at IS NULL OR ends_at IS NULL OR ends_at>starts_at)'),
  ('events', 'events_publication_complete', 'CHECK(status=''draft'' OR (length(trim(title))>0 AND starts_at IS NOT NULL AND ends_at IS NOT NULL AND length(destination_url)>0))'),
  ('event_rsvps', 'event_rsvps_event_org_fk', 'FOREIGN KEY(event_id,organisation_id) REFERENCES events(id,organisation_id)'),
  ('event_rsvps', 'event_rsvps_user_fk', 'FOREIGN KEY(user_id) REFERENCES users(id)'),
  ('event_hub_settings', 'event_settings_organisation_fk', 'FOREIGN KEY(organisation_id) REFERENCES organisations(id)'),
  ('event_hub_settings', 'event_settings_featured_org_fk', 'FOREIGN KEY(featured_event_id,organisation_id) REFERENCES events(id,organisation_id)'),
  ('event_notification_outbox', 'events_outbox_event_org_fk', 'FOREIGN KEY(event_id,organisation_id) REFERENCES events(id,organisation_id)'),
  ('event_notification_outbox', 'events_outbox_user_fk', 'FOREIGN KEY(user_id) REFERENCES users(id)'),
  ('event_notification_outbox', 'events_outbox_kind_valid', 'CHECK(kind IN (''reminder'',''updated'',''cancelled''))'),
  ('event_notification_outbox', 'events_outbox_state_valid', 'CHECK(state IN (''pending'',''delivered'',''suppressed''))')
 ) AS constraints(table_name, constraint_name, definition)
 LOOP
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = item.table_name::regclass AND conname = item.constraint_name) THEN
   EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I %s', item.table_name, item.constraint_name, item.definition);
  END IF;
 END LOOP;
END $$;
UPDATE permissions p SET permission_list = p.permission_list ||
 (CASE WHEN p.permission_list ? 'can_view_events' THEN '{}'::jsonb ELSE jsonb_build_object('can_view_events',r.name IN ('Owner','Administrator','Manager','Project Lead','User')) END) ||
 (CASE WHEN p.permission_list ? 'can_manage_events' THEN '{}'::jsonb ELSE jsonb_build_object('can_manage_events',r.name IN ('Owner','Administrator','Manager','Project Lead')) END) ||
 (CASE WHEN p.permission_list ? 'can_feature_events' THEN '{}'::jsonb ELSE jsonb_build_object('can_feature_events',r.name IN ('Owner','Administrator','Project Lead')) END)
 FROM org_roles r WHERE r.id::text=p.role_id::text AND r.is_default=true;

CREATE TABLE IF NOT EXISTS events (
 id uuid PRIMARY KEY, organisation_id uuid NOT NULL REFERENCES organisations(id), created_by uuid NOT NULL REFERENCES users(id), operation_id uuid NOT NULL,
 host_id uuid NOT NULL REFERENCES users(id), title varchar(160) NOT NULL DEFAULT '', description text NOT NULL DEFAULT '', category varchar(80) NOT NULL DEFAULT '',
 status varchar(16) NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','published','cancelled')), starts_at timestamptz, ends_at timestamptz,
 organiser_time_zone varchar(64) NOT NULL DEFAULT 'Africa/Lagos', destination_url varchar(2048) NOT NULL DEFAULT '', revision bigint NOT NULL DEFAULT 1, schedule_version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT idx_events_create UNIQUE(organisation_id,created_by,operation_id), CONSTRAINT events_org_identity UNIQUE(id,organisation_id),
 CONSTRAINT events_schedule_order CHECK(starts_at IS NULL OR ends_at IS NULL OR ends_at>starts_at),
 CONSTRAINT events_publication_complete CHECK(status='draft' OR (length(trim(title))>0 AND starts_at IS NOT NULL AND ends_at IS NOT NULL AND length(destination_url)>0))
);
CREATE INDEX IF NOT EXISTS idx_events_org_schedule ON events(organisation_id,status,starts_at,id);
CREATE TABLE IF NOT EXISTS event_rsvps (
 event_id uuid NOT NULL,user_id uuid NOT NULL REFERENCES users(id),organisation_id uuid NOT NULL,going boolean NOT NULL DEFAULT false,
 reminder_eligible boolean NOT NULL DEFAULT false,schedule_version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,user_id),FOREIGN KEY(event_id,organisation_id) REFERENCES events(id,organisation_id)
);
CREATE INDEX IF NOT EXISTS idx_event_rsvps_member ON event_rsvps(organisation_id,user_id,going);
CREATE TABLE IF NOT EXISTS event_hub_settings (
 organisation_id uuid PRIMARY KEY REFERENCES organisations(id),enabled boolean NOT NULL DEFAULT false,featured_event_id uuid,revision bigint NOT NULL DEFAULT 0,updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(featured_event_id,organisation_id) REFERENCES events(id,organisation_id)
);
CREATE TABLE IF NOT EXISTS event_notification_outbox (
 id uuid PRIMARY KEY,event_id uuid NOT NULL,organisation_id uuid NOT NULL,user_id uuid NOT NULL REFERENCES users(id),kind varchar(16) NOT NULL CHECK(kind IN ('reminder','updated','cancelled')),
 delivery_key varchar(256) NOT NULL UNIQUE,due_at timestamptz NOT NULL,revision bigint NOT NULL,schedule_version bigint NOT NULL,state varchar(16) NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','delivered','suppressed')),
 attempts integer NOT NULL DEFAULT 0,lease_until timestamptz,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,organisation_id) REFERENCES events(id,organisation_id)
);
CREATE INDEX IF NOT EXISTS idx_events_due ON event_notification_outbox(state,due_at);
UPDATE permissions p SET permission_list = p.permission_list ||
 (CASE WHEN p.permission_list ? 'can_view_events' THEN '{}'::jsonb ELSE jsonb_build_object('can_view_events',r.name IN ('Owner','Administrator','Manager','Project Lead','User')) END) ||
 (CASE WHEN p.permission_list ? 'can_manage_events' THEN '{}'::jsonb ELSE jsonb_build_object('can_manage_events',r.name IN ('Owner','Administrator','Manager','Project Lead')) END) ||
 (CASE WHEN p.permission_list ? 'can_feature_events' THEN '{}'::jsonb ELSE jsonb_build_object('can_feature_events',r.name IN ('Owner','Administrator','Project Lead')) END)
 FROM org_roles r WHERE r.id::text=p.role_id::text AND r.is_default=true;

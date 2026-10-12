# Events Hub deployment guide

## Enablement

Apply `db/migrations/000003_events_hub.up.sql` through the existing migration workflow before enabling the feature. `event_hub_settings.enabled` defaults to false. Enable only the approved testing organisation:

```sql
INSERT INTO event_hub_settings (organisation_id, enabled, revision, updated_at)
VALUES ('<testing-organisation-uuid>', true, 0, now())
ON CONFLICT (organisation_id) DO UPDATE SET enabled = true;
```

Use existing authentication and database configuration. Default organisation roles receive event permissions; explicit denials and custom grants remain intact. Review grants before activation.

## Behaviour

Authenticated endpoints live under `/api/v1/organisations/:org_id/events`. Members discover sessions, RSVP and resolve Join inside the server-controlled time window. Organisers manage drafts, publication and cancellation; leads feature an event. One-off schedules store UTC and display Africa/Lagos time. External HTTPS meeting destinations support the first release.

Reminders and update/cancellation notices persist in the existing in-app inbox. The outbox reconciler runs every 30 seconds and River retries delivery. Browser push is outside this implementation. Disabling the organisation setting closes access and suppresses pending notices. The down migration removes event data and must not be used as an ordinary feature-disable mechanism.

## Acceptance checks

Use a dedicated PostgreSQL test database and set `EVENTS_TEST_DATABASE_URL`; run `go test ./tests/test_events -count=1` and `go test -race ./tests/test_events -count=1`. Database tests skip without this variable; skipped tests do not establish database correctness. Run `go build ./...`, `go vet ./...`, and the broader repository suite with dedicated service configuration.

Before activation, verify deployed frontend/backend revisions, migration completion and the active notice executor. Use host, lead, member and non-member accounts in separate browsers to check publication, discovery, RSVP persistence, reminders with the browser closed, Join boundaries, rescheduling, cancellation and access revocation. Confirm permission isolation across organisations.

No deployment, push or destination-branch merge is performed by this guide.

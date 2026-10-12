# Events Hub: team testing handoff

Branch: `feat/events-hub-backend`. Local implementation; no deployment performed.

## Enablement

Apply migration `db/migrations/000003_events_hub.up.sql` through the repository migration workflow. The feature defaults off for every organisation. After selecting an approved testing organisation, enable only that organisation:

```sql
INSERT INTO event_hub_settings (organisation_id, enabled, revision)
VALUES ('<testing-organisation-uuid>', true, 0)
ON CONFLICT (organisation_id) DO UPDATE SET enabled = true;
```

Use the existing authenticated API and database configuration. Do not point disposable tests at a shared database. Default organisation roles receive event permissions; explicit denials and custom roles retain their grants. Review grants before enabling.

## Behaviour and workaround

Routes live under `/api/v1/organisations/:org_id/events`. Capabilities gate discovery. Members RSVP and resolve Join during the server-controlled window; organisers manage drafts, publication and cancellation; leads feature an event. One-off schedules use UTC storage and Africa/Lagos display. External HTTPS meeting links provide the meeting destination until advance Buzz scheduling is supported.

Reminders and update/cancellation notices persist in the existing in-app inbox. The outbox reconciler runs every 30 seconds and River retries delivery. OneSignal push delivery is not claimed. Disable the organisation setting to close access and suppress notices; keep durable rows for investigation. Destructive down migration removes event data and should only be used on disposable environments.

## Verification evidence

Before final review fixes, 14 focused PostgreSQL Events Hub tests passed. Review found and fixed concurrent creation retry lookup, pending schedule notices lost after a text edit, and explicit false event permissions omitted during serialization. Added three regression tests; the serialization regression failed before its fix and passed afterward. After the approval setting was updated, all 17 focused PostgreSQL tests passed. The backend build, go vet ./..., and selected existing permission tests also passed. All 17 tests also passed with Go race detection enabled. Formatting and git diff whitespace checks passed.

The broader repository suite was attempted with a 180-second outer limit and could not complete: existing tests lack their dedicated service configuration in this isolated checkout (including an empty database port). No live app.env was copied. These failures are not reported as passing.

For disposable PostgreSQL only, set `EVENTS_TEST_DATABASE_URL` and run `go test ./tests/test_events -count=1`, then `go test -race ./tests/test_events -count=1`. Without the variable, database tests skip; that is not database verification. Run `go build ./...`, `go vet ./...` and the repository suite with a proper dedicated test environment before shared deployment.

Frontend uses its own feature branch and requires a maintainer-approved route allowlist update before upstream review. No protected configuration was changed. Replace descriptive branch names with approved ticket branches before submission where required.

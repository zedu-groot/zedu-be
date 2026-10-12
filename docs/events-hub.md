# Events Hub deployment guide

## Enablement

Apply `db/migrations/000003_events_hub.up.sql` through the existing migration workflow. Events is ON by default for this staging integration, as requested by the team. An authenticated organisation member's first Events capability check creates enabled settings automatically; no manual activation SQL is needed. Existing explicit disable settings remain respected.

Use existing authentication and database configuration. Default organisation roles receive event permissions; explicit denials and custom grants remain intact. The switch remains available to disable the feature through the organisation settings record.

## Behaviour

Authenticated endpoints live under `/api/v1/organisations/:org_id/events`. Members discover sessions, RSVP and resolve Join inside the server-controlled time window. Organisers manage drafts, publication and cancellation; leads feature an event. One-off schedules store UTC and display Africa/Lagos time. External HTTPS meeting destinations support the first release.

Reminders and update/cancellation notices persist in the existing in-app inbox. The outbox reconciler runs every 30 seconds and River retries delivery. Browser push is outside this implementation. Disabling the organisation setting closes access and suppresses pending notices. The down migration removes event data and must not be used as an ordinary feature-disable mechanism.

## Acceptance checks

Use a dedicated PostgreSQL test database and set `EVENTS_TEST_DATABASE_URL`; run `go test ./tests/test_events -count=1` and `go test -race ./tests/test_events -count=1`. Database tests skip without this variable; skipped tests do not establish database correctness. Run `go build ./...`, `go vet ./...`, and the broader repository suite with dedicated service configuration.

Before activation, verify deployed frontend/backend revisions, migration completion and the active notice executor. Use host, lead, member and non-member accounts in separate browsers to check publication, discovery, RSVP persistence, reminders with the browser closed, Join boundaries, rescheduling, cancellation and access revocation. Confirm permission isolation across organisations.

No deployment, push or destination-branch merge is performed by this guide.

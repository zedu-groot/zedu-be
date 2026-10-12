package cronjobs

import (
	"context"

	"github.com/hngprojects/telex_be/external/request"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	events "github.com/hngprojects/telex_be/services/events"
	"github.com/riverqueue/river"
)

func ProcessEventsOutbox(extReq request.ExternalRequest, db storage.Database) {
	service := events.NewService(db.Postgresql, nil)
	input := events.ReconcileInput{Limit: 100}
	if db.River != nil {
		input.Enqueue = func(ctx context.Context, id string) error {
			_, err := db.River.Insert(ctx, models.EventNotificationJobArgs{OutboxID: id}, &river.InsertOpts{MaxAttempts: 5})
			return err
		}
	}
	if err := service.ReconcileDueNotices(context.Background(), input); err != nil {
		extReq.Logger.Error("events outbox reconciliation failed: %v", err)
	}
}

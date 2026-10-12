package riverqueueBg

import (
	"context"
	"fmt"

	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/centrifuge"
	events "github.com/hngprojects/telex_be/services/events"
	"github.com/hngprojects/telex_be/utility"
	"github.com/riverqueue/river"
	"gorm.io/gorm"
)

type EventsNoticeWorker struct {
	river.WorkerDefaults[models.EventNotificationJobArgs]
	db     *gorm.DB
	logger *utility.Logger
}

func (w *EventsNoticeWorker) Work(ctx context.Context, job *river.Job[models.EventNotificationJobArgs]) error {
	service := events.NewService(w.db, nil)
	service.PublishNotice = func(ctx context.Context, n models.OneSignalNotification) error {
		payload := map[string]any{"notification_type": n.Payload["notification_type"], "notification_id": n.ID, "data": n.Payload, "section": "events"}
		err := centrifuge.PublishChannel(w.logger, fmt.Sprintf("%s/%s", *n.OrgID, n.UserID), payload)
		if err != nil {
			w.logger.Error("event inbox saved; live publication unavailable: %v", err)
		}
		return err
	}
	return service.DeliverNotice(ctx, events.NoticeInput{OutboxID: job.Args.OutboxID})
}

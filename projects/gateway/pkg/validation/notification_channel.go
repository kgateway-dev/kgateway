package validation

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
)

func MakeNotificationChannel(ctx context.Context, stream validation.ProxyValidationService_NotifyOnResyncClient) (<-chan struct{}, error) {
	notifications := make(chan struct{}, 1)

	// we expect a notification right away as our "ack"
	_, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	go func() {
		logger := contextutils.LoggerFrom(contextutils.WithLogger(ctx, "validation-resync-notifications"))
		defer close(notifications)
		defer logger.Infof("shutting down notification channel")
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			notification, err := stream.Recv()
			if err != nil {
				logger.Errorw("error reading from stream", zap.Error(err))
				continue
			}

			logger.Debug("received", zap.Any("notification", notification))

			select {
			case <-ctx.Done():
				return
			case notifications <- struct{}{}:
			default:
				logger.Debug("dropping notification")
			}
		}
	}()

	return notifications, nil
}

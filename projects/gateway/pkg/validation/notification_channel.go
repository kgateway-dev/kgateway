package validation

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
	"go.uber.org/zap"
)

func MakeNotificationChannel(ctx context.Context, client validation.ProxyValidationServiceClient) (<-chan struct{}, error) {
	notifications := make(chan struct{}, 1)

	logger := contextutils.LoggerFrom(contextutils.WithLogger(ctx, "validation-resync-notifications"))

	stream, err := startNotificationStream(ctx, client, logger)
	if err != nil {
		return nil, err
	}

	go func() {
		logger.Infof("starting notification channel")
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
				select {
				case <-ctx.Done():
					return
				default:
				}
				logger.Errorw("error reading from stream. attempting to establish new stream.", zap.Error(err))
				stream, err = startNotificationStream(ctx, client, logger)
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
					}
					// TODO(kdorosh) be noiser here when we fail. bubble up onto proxy status perhaps
					logger.Errorw("failed to resume notifications. Gateway will no longer receive validation resync notifications from Gloo.", zap.Error(err))
					return
				}
				continue
			}
			logger.Debugf("received notification", zap.Any("notification", notification), zap.Error(err))

			select {
			case notifications <- struct{}{}:
				logger.Debugf("sent notification to notifications channel")
			default:
				logger.Debug("dropping notification")
			}
		}
	}()

	return notifications, nil
}

func startNotificationStream(ctx context.Context, client validation.ProxyValidationServiceClient, logger *zap.SugaredLogger) (validation.ProxyValidationService_NotifyOnResyncClient, error) {
	// fail if we cannot establish notifications from gloo
	stream, err := client.NotifyOnResync(ctx, &validation.NotifyOnResyncRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to stream notifications from validation server.")
	}

	// we expect a notification right away as our "ack"
	if _, err = stream.Recv(); err != nil {
		return nil, err
	}

	contextutils.LoggerFrom(ctx).Debug("validation ACK received")

	return stream, nil
}

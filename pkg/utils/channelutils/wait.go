package channelutils

import (
	"context"
	"time"
)

func WaitForReady(ctx context.Context, timeout time.Duration, readychans ...<-chan struct{}) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// wait for everything to finish warming up:
	for _, chans := range readychans {
		select {
		case <-chans:
			// wait for component to become ready
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

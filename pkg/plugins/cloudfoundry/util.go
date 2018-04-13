package cloudfoundry

import (
	"context"
	"time"
)

func ResyncLoop(ctx context.Context, stop <-chan struct{}, resync func(), resyncDuration time.Duration) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(resyncDuration)
	defer ticker.Stop()

	if stop != nil {
		go func(cancel context.CancelFunc) {
			<-stop
			cancel()
		}(cancel)
	}

	// first time resync
	resync()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resync()
		}
	}
}

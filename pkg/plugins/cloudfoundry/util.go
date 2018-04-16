package cloudfoundry

import (
	"context"
	"time"
)

func ResyncLoop(ctx context.Context, stop <-chan struct{}, resync func(), resyncDuration time.Duration) {
	ctx, cancel := MakeStopCancelContext(ctx, stop)
	defer cancel()

	ticker := time.NewTicker(resyncDuration)
	defer ticker.Stop()

	ResyncLoopWithTicker(ctx, resync, ticker.C)
}

func MakeStopCancelContext(ctx context.Context, stop <-chan struct{}) (context.Context, context.CancelFunc) {
	if stop != nil {
		ctx, cancel := context.WithCancel(ctx)
		go func(cancel context.CancelFunc) {
			<-stop
			cancel()
		}(cancel)
		return ctx, cancel
	}
	return ctx, func() {}

}

func ResyncLoopWithTicker(ctx context.Context, resync func(), ticker <-chan time.Time) {
	// first time resync
	resync()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
			resync()
		}
	}
}

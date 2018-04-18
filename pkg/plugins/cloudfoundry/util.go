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

	ResyncLoopWithTicker(ctx, resync, ticker.C, nil)
}

func ResyncLoopWithKick(ctx context.Context, stop <-chan struct{}, resync func(), resyncDuration time.Duration, kick <-chan struct{}) {
	ctx, cancel := MakeStopCancelContext(ctx, stop)
	defer cancel()

	ticker := time.NewTicker(resyncDuration)
	defer ticker.Stop()

	ResyncLoopWithTicker(ctx, resync, ticker.C, kick)
}

func MakeStopCancelContext(ctx context.Context, stop <-chan struct{}) (context.Context, context.CancelFunc) {
	if stop == nil {
		return ctx, func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	go func(cancel context.CancelFunc) {
		<-stop
		cancel()
	}(cancel)
	return ctx, cancel
}

func ResyncLoopWithTicker(ctx context.Context, resync func(), ticker <-chan time.Time, kick <-chan struct{}) {
	// first time resync
	resync()

	for {
		select {
		case <-ctx.Done():
			return
		case <-kick:
			resync()
		case <-ticker:
			resync()
		}
	}
}

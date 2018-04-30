package backoff

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/log"
)

// Default values for ExponentialBackOff.
const (
	defaultInitialInterval = 500 * time.Millisecond
	defaultMaxElapsedTime  = 60 * time.Second

	backoffCap = 15 * time.Minute
)

func WithBackoff(fn func() error, stop chan struct{}) error {
	// first try
	if err := fn(); err == nil {
		return nil
	}
	tilNextRetry := defaultInitialInterval
	var elapsed time.Duration
	for {
		select {
		// stopped by another goroutine
		case <-stop:
			return nil
		case <-time.After(tilNextRetry):
			elapsed += tilNextRetry
			tilNextRetry *= 2
			err := fn()
			if err == nil {
				return nil
			}
			if elapsed >= defaultMaxElapsedTime {
				return err
			}
		}
	}
}

func WithBackoffWithContext(fn func() error, maxBackoff time.Duration, ctx context.Context) error {
	// first try
	var err error
	if err = fn(); err == nil {
		return nil
	}
	tilNextRetry := defaultInitialInterval
	var elapsed time.Duration
	for {
		select {
		// stopped by another goroutine
		case <-ctx.Done():
			return err
		case <-time.After(tilNextRetry):
			elapsed += tilNextRetry

			tilNextRetry *= 2

			if tilNextRetry >= backoffCap {
				tilNextRetry = backoffCap
				log.Warnf("reached maximum backoff with error: %v", err)
			}

			err = fn()
			if err == nil {
				return nil
			}
		}
	}
}

// does not return until success
func UntilSuccess(fn func() error, ctx context.Context) {
	// first try
	if err := fn(); err == nil {
		return
	}
	tilNextRetry := defaultInitialInterval
	for {
		select {
		// stopped by another goroutine
		case <-ctx.Done():
			return
		case <-time.After(tilNextRetry):
			tilNextRetry *= 2
			err := fn()
			if err == nil {
				return
			}
			if tilNextRetry >= backoffCap {
				tilNextRetry = backoffCap
				log.Warnf("reached maximum backoff with error: %v", err)
			}
		}
	}
}

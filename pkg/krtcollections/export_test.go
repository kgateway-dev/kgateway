package krtcollections

import "time"

// SetXdsFirstConnectDelayForTest overrides the first-connect delay slept on
// new xDS streams and returns a function that restores the previous value.
// The underlying value is atomic, so the override and restore cannot race
// stream goroutines from a live test server that are still running.
func SetXdsFirstConnectDelayForTest(d time.Duration) (restore func()) {
	xdsFirstConnectDelay() // force the lazy env read so restore semantics are stable
	prev := xdsFirstConnectDelayNanos.Swap(int64(d))
	return func() { xdsFirstConnectDelayNanos.Store(prev) }
}

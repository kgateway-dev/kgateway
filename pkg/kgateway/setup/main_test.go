package setup_test

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// These tests drive live in-process ADS servers. Run them with the xDS
	// first-connect grace period disabled (unless the environment explicitly
	// sets one): the delay is a mitigation, not a correctness requirement, so
	// the suite's eventually-consistent assertions must hold against the raw
	// reconnect race the delay merely narrows — running at zero both surfaces
	// bugs the grace period would mask and removes a fixed per-stream second
	// from the suite's runtime. The delay contract itself is pinned by
	// TestFirstConnectDelayGatesFirstRequestPerStream in pkg/krtcollections.
	// The variable is read lazily on first stream connect, so setting it here
	// (after package initialization, before any server runs) is effective.
	if os.Getenv("KGW_XDS_FIRST_CONNECT_DELAY") == "" {
		os.Setenv("KGW_XDS_FIRST_CONNECT_DELAY", "0")
	}
	os.Exit(m.Run())
}

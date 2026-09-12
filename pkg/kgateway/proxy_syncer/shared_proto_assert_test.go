package proxy_syncer

import (
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
)

// TestMain forces the shared-proto mutation tripwire on for every test in this
// package. The integration tests here run the real cluster and endpoint producer
// collections (NewPerClientEnvoyClusters, NewPerClientEnvoyEndpoints) through
// snapshotPerClient, and the local-cluster tests publish
// NewPerClientLocalClusterEndpoints rows through the same ResourceWithTTL sink,
// so with the tripwire armed, any code path that mutates a shared proto between
// creation and publication panics in CI instead of silently corrupting sibling
// clients. TestNewPerClientEnvoyClusters_ArmedTripwireCatchesBaseMutation is the
// negative control proving the armed tripwire fires on a real row. Test fixtures
// that wrap protos while the flag is on are verified too; rows built from the
// zero-value wrapper are skipped.
func TestMain(m *testing.M) {
	sharedproto.AssertImmutability = true
	os.Exit(m.Run())
}

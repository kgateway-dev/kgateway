package proxy_syncer

import (
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
)

// TestMain forces the shared-proto mutation tripwire on for every test in this
// package. The integration tests here run the real producer collections
// (NewPerClientEnvoyClusters, NewPerClientEnvoyEndpoints,
// NewPerClientLocalClusterEndpoints) through snapshotPerClient, so with the
// tripwire armed, any code path that mutates a shared proto between creation
// and snapshot assembly panics in CI instead of silently corrupting sibling
// clients. Test fixtures that wrap protos while the flag is on are verified
// too; rows built from the zero-value wrapper are skipped.
func TestMain(m *testing.M) {
	sharedproto.AssertImmutability = true
	os.Exit(m.Run())
}

package proxy_syncer

import (
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
)

// TestMain forces the shared-proto mutation tripwire on for every test in this
// package. Integration tests run the real sparse cluster producer through
// snapshot assembly, so mutation between wrapping and publication panics.
func TestMain(m *testing.M) {
	sharedproto.AssertImmutability = true
	os.Exit(m.Run())
}

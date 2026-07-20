//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/listenerset"
)

// TestListenerSet tests ListenerSet support.
func TestListenerSet(t *testing.T) {
	listenerset.Run(t, e2e.DefaultInstallationFactory)
}

//go:build e2e

package basicrouting

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"testing"

	confsuite "sigs.k8s.io/gateway-api/conformance/utils/suite"

	"github.com/kgateway-dev/kgateway/v2/test/sigs_gateway_conformance/common"
)

//go:embed testdata
var manifestFS embed.FS

const (
	gatewayClassName = "kgateway"
	testNamespace    = "kgateway-conformance-test"
	gatewayName      = "conformance-gateway"
	routeName        = "basicrouting-route"
)

// suite holds the conformance test suite shared by all tests in the package.
var suite *confsuite.ConformanceTestSuite

// TestMain wires up a Gateway API conformance ConformanceTestSuite. Individual
// manifests applied through suite.Applier register t.Cleanup hooks, so tests do
// not need to manage manifest teardown explicitly.
func TestMain(m *testing.M) {
	if err := common.SetupConformanceSuite(gatewayClassName, []fs.FS{manifestFS}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set up conformance suite: %v\n", err)
		os.Exit(1)
	}
	suite = common.GetSuite()
	os.Exit(m.Run())
}

//go:build e2e

package agentgateway

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	namespace = "agentgateway-base"
)

var (
	// kgateway managed deployment for the agentgateway with basic HTTPRoute
	httpRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agw-http-route.yaml")
	// kgateway managed deployment for the agentgateway with basic TCPRoute
	tcpRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agw-tcp-route.yaml")

	testCases = map[string]*base.TestCase{
		"TestAgentgatewayHTTPRoute": {
			Manifests: []string{httpRouteManifest},
		},
		"TestAgentgatewayTCPRoute": {
			Manifests: []string{tcpRouteManifest},
			// TCPRoutes are experimental (v0.3.0+)
			// Don't specify MinGwApiVersion here to avoid test framework skip logic
			// The suite-level GwApiRequireRouteNames requirement ensures proper Gateway API version
		},
	}

	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: namespace,
		},
	}
)

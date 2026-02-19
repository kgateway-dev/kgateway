//go:build e2e

package parallelcontrollers

import (
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// Template manifests
	envoyGatewayTemplate = filepath.Join(fsutils.MustGetThisDir(), "testdata", "envoy-gateway.yaml")
	agwGatewayTemplate   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agw-gateway.yaml")

	// Object metadata for shared resources
	httpbinObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "default",
	}

	// Object metadata for envoy-only test
	envoyGwEnvoyOnlyMeta = metav1.ObjectMeta{
		Name:      "envoy-gw-envoy-only",
		Namespace: "default",
	}
	envoyRouteEnvoyOnlyMeta = metav1.ObjectMeta{
		Name:      "envoy-route-envoy-only",
		Namespace: "default",
	}
	agwGwEnvoyOnlyMeta = metav1.ObjectMeta{
		Name:      "agw-gw-envoy-only",
		Namespace: "default",
	}

	// Object metadata for agentgateway-only test
	envoyGwAgwOnlyMeta = metav1.ObjectMeta{
		Name:      "envoy-gw-agw-only",
		Namespace: "default",
	}
	agwGwAgwOnlyMeta = metav1.ObjectMeta{
		Name:      "agw-gw-agw-only",
		Namespace: "default",
	}
	agwRouteAgwOnlyMeta = metav1.ObjectMeta{
		Name:      "agw-route-agw-only",
		Namespace: "default",
	}

	// Object metadata for both-enabled test
	envoyGwBothEnabledMeta = metav1.ObjectMeta{
		Name:      "envoy-gw-both-enabled",
		Namespace: "default",
	}
	envoyRouteBothEnabledMeta = metav1.ObjectMeta{
		Name:      "envoy-route-both-enabled",
		Namespace: "default",
	}
	agwGwBothEnabledMeta = metav1.ObjectMeta{
		Name:      "agw-gw-both-enabled",
		Namespace: "default",
	}
	agwRouteBothEnabledMeta = metav1.ObjectMeta{
		Name:      "agw-route-both-enabled",
		Namespace: "default",
	}
)

// transformManifest replaces placeholders in the manifest with actual values
func transformManifest(gatewayName, routeName, hostname string) func(string) string {
	return func(content string) string {
		content = strings.ReplaceAll(content, "GATEWAY_NAME", gatewayName)
		content = strings.ReplaceAll(content, "ROUTE_NAME", routeName)
		content = strings.ReplaceAll(content, "HOSTNAME", hostname)
		return content
	}
}

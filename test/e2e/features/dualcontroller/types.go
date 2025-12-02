//go:build e2e

package dualcontroller

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

	// Object metadata for phase 1
	envoyGwPhase1Meta = metav1.ObjectMeta{
		Name:      "envoy-gw-phase1",
		Namespace: "default",
	}
	agwGwPhase1Meta = metav1.ObjectMeta{
		Name:      "agw-gw-phase1",
		Namespace: "default",
	}

	// Object metadata for phase 2
	envoyGwPhase2Meta = metav1.ObjectMeta{
		Name:      "envoy-gw-phase2",
		Namespace: "default",
	}
	agwGwPhase2Meta = metav1.ObjectMeta{
		Name:      "agw-gw-phase2",
		Namespace: "default",
	}

	// Object metadata for phase 3
	envoyGwPhase3Meta = metav1.ObjectMeta{
		Name:      "envoy-gw-phase3",
		Namespace: "default",
	}
	agwGwPhase3Meta = metav1.ObjectMeta{
		Name:      "agw-gw-phase3",
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

package deployer

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	pkgdeployer "github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

// Envoy reads its bootstrap ConfigMap only at startup, so the proxy pod template
// must carry a checksum of the ConfigMap that changes whenever the bootstrap changes.
func TestEnvoyDeploymentConfigChecksum(t *testing.T) {
	chart, err := loadChart(helm.EnvoyHelmChart)
	require.NoError(t, err)
	d := pkgdeployer.NewDeployer("test-controller", schemes.GatewayScheme(), nil, chart, nil, nil)

	checksum := func(vals map[string]any) string {
		t.Helper()
		objs, err := d.RenderChartToObjects("default", "gw", vals)
		require.NoError(t, err)
		for _, obj := range objs {
			if dep, ok := obj.(*appsv1.Deployment); ok {
				sum := dep.Spec.Template.Annotations["kgateway.dev/checksum-config"]
				require.NotEmpty(t, sum, "pod template should have a kgateway.dev/checksum-config annotation")
				return sum
			}
		}
		require.FailNow(t, "no Deployment rendered")
		return ""
	}
	withLogFormat := func(text string) map[string]any {
		return map[string]any{"gateway": map[string]any{
			"name":             "gw",
			"fullnameOverride": "gw",
			"gatewayName":      "gw",
			"image":            map[string]any{"registry": "cr.kgateway.dev", "repository": "envoy-wrapper", "tag": "test"},
			"logFormat":        map[string]any{"text": text},
		}}
	}

	base := checksum(withLogFormat("[%Y-%m-%d %T.%e][%l] %v"))
	require.Equal(t, base, checksum(withLogFormat("[%Y-%m-%d %T.%e][%l] %v")),
		"checksum should be stable for identical input")
	require.NotEqual(t, base, checksum(withLogFormat("%v")),
		"checksum should change when the bootstrap changes")

	// Envoy reloads the xDS CA live, so rotating it must not roll the pods.
	withCA := func(caCert string) map[string]any {
		vals := withLogFormat("%v")
		vals["gateway"].(map[string]any)["xds"] = map[string]any{
			"tls": map[string]any{"enabled": true, "caCert": caCert},
		}
		return vals
	}
	require.Equal(t, checksum(withCA("old-ca")), checksum(withCA("new-ca")),
		"checksum should not change when only the xDS CA changes")
}

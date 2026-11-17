package deployer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoregistry"
	corev1 "k8s.io/api/core/v1"

	pkgdeployer "github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

// The controller parses the generated bootstrap to apply overlays, which needs every
// `@type` in it linked into the controller. Render the bootstrap with every option that
// adds a `@type` and parse it; a template change that adds an unlinked type fails here.
func TestGeneratedBootstrapParses(t *testing.T) {
	// This test proves nothing if the test binary links every Envoy type, as
	// pkg/utils/filter_types does. A contrib type is linked only through it.
	_, err := protoregistry.GlobalTypes.FindMessageByName("envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker")
	require.ErrorIs(t, err, protoregistry.NotFound,
		"this test binary links pkg/utils/filter_types; keep its imports out of pkg/kgateway/deployer tests")

	// Render the chart with every option that adds a `@type` to the bootstrap.
	chart, err := loadChart(helm.EnvoyHelmChart)
	require.NoError(t, err)
	d := pkgdeployer.NewDeployer("test-controller", schemes.GatewayScheme(), nil, chart, nil, nil)
	objs, err := d.RenderChartToObjects("default", "gw", map[string]any{"gateway": map[string]any{
		"name":                              "gw",
		"fullnameOverride":                  "gw",
		"gatewayName":                       "gw",
		"image":                             map[string]any{"registry": "cr.kgateway.dev", "repository": "envoy-wrapper", "tag": "test"},
		"logFormat":                         map[string]any{"json": map[string]any{"message": "%LOCAL_REPLY_BODY%"}},
		"xds":                               map[string]any{"host": "kgateway.kgateway-system.svc", "port": 9977, "tls": map[string]any{"enabled": true, "caCert": "ca"}},
		"istio":                             map[string]any{"enabled": true},
		"dnsResolver":                       map[string]any{"udpMaxQueries": 50},
		"maxDownstreamConnections":          1000,
		"enableReadinessProbeProxyProtocol": true,
		"readinessProbe":                    map[string]any{"httpGet": map[string]any{"path": "/envoy-hc", "port": 8082}},
		"stats":                             map[string]any{"enabled": true, "enableStatsRoute": true, "matcher": map[string]any{"inclusionList": []any{map[string]any{"prefix": "cluster."}}}},
	}})
	require.NoError(t, err)

	// Parse the bootstrap from the ConfigMap.
	var envoyYaml string
	for _, obj := range objs {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			envoyYaml = cm.Data["envoy.yaml"]
		}
	}
	require.NotEmpty(t, envoyYaml, "no ConfigMap with envoy.yaml rendered")
	bs, err := bootstrappatch.ParseBootstrap([]byte(envoyYaml))
	require.NoError(t, err, "the generated bootstrap must parse in the controller; link the missing type in pkg/kgateway/bootstrappatch/types.go")

	// Check that the options took effect, so the test covers their `@type`s.
	require.NotNil(t, bs.GetTypedDnsResolverConfig(), "dnsResolver should render")
	require.NotEmpty(t, bs.GetOverloadManager().GetResourceMonitors(), "maxDownstreamConnections should render")
	require.Len(t, bs.GetStaticResources().GetListeners(), 2, "stats should render prometheus_listener")
}

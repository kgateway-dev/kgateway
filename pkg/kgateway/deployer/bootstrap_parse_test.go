package deployer

import (
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	corev1 "k8s.io/api/core/v1"

	pkgdeployer "github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// The controller parses the generated bootstrap and overlays, which needs every `@type`
// they use linked into the controller. bootstrapoverlay links every go-control-plane type
// and the cncf/xds TypedStruct wrappers. Check that a type from each source is linked,
// then render the bootstrap with every option that adds a `@type` and parse it.
func TestGeneratedBootstrapParses(t *testing.T) {
	// A contrib type, an extension the generated bootstrap does not use, and both
	// TypedStruct wrappers are linked.
	for _, name := range []protoreflect.FullName{
		"envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker",
		"envoy.extensions.stat_sinks.open_telemetry.v3.SinkConfig",
		"udpa.type.v1.TypedStruct",
		"xds.type.v3.TypedStruct",
	} {
		_, err := protoregistry.GlobalTypes.FindMessageByName(name)
		require.NoError(t, err, "%s should be linked through pkg/utils/filter_types", name)
	}

	// Parse the bootstrap rendered with every option.
	bs := renderFullBootstrap(t)

	// Check that the options took effect, so the test covers their `@type`s.
	require.NotNil(t, bs.GetTypedDnsResolverConfig(), "dnsResolver should render")
	require.NotEmpty(t, bs.GetOverloadManager().GetResourceMonitors(), "maxDownstreamConnections should render")
	require.Len(t, bs.GetStaticResources().GetListeners(), 2, "stats should render prometheus_listener")
}

// The controller validates a merged bootstrap with Envoy, prepared as the proxy pod
// runs it. The generated bootstrap with every option, including xDS TLS with its
// file-based SDS and a JSON log format, must pass.
func TestGeneratedBootstrapValidates(t *testing.T) {
	// Validate the bootstrap rendered with every option with the bundled Envoy.
	bs := renderFullBootstrap(t)
	require.NoError(t, validator.NewDocker().(validator.BootstrapValidator).ValidateBootstrap(t.Context(), bs))
}

// renderFullBootstrap renders the envoy chart with every option that adds a `@type`
// or a file reference to the bootstrap, and parses the bootstrap.
func renderFullBootstrap(t *testing.T) *envoybootstrapv3.Bootstrap {
	t.Helper()

	// Render the chart.
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
	bs, err := bootstrapoverlay.ParseBootstrap([]byte(envoyYaml))
	require.NoError(t, err, "the generated bootstrap must parse in the controller")
	return bs
}

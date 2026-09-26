package waypoint_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/waypoint"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
)

// exampleGw is used in most tests, but we may want to have
// multiple Gateways in the input at some point and target a specific
// one for translation results
var exampleGw = types.NamespacedName{Name: "example-waypoint", Namespace: "infra"}

var cases = []struct {
	name string
	file string
	gw   types.NamespacedName
	skip string
}{
	{"Service use-waypoint", "svc-use-waypoint", exampleGw, ""},
	{"ServiceEntry use-waypoint", "se-use-waypoint", exampleGw, ""},
	{"Namespace use-waypoint", "ns-use-waypoint", exampleGw, ""},
	{"HTTPRoute on Gateway", "httproute-gateway", exampleGw, ""},
	{"HTTPRoute on Service", "httproute-svc", exampleGw, ""},
	{"HTTPRoute on ServiceEntry", "httproute-se", exampleGw, ""},
	{"HTTPRoute on ServiceEntry via Hostname", "httproute-se-hostname", exampleGw, ""},
	{"Authz Policies", "authz", exampleGw, ""},
	{"Authz Policies - Gateway Ref", "authz-gateway-ref", exampleGw, ""},
	{"Authz Policies - Gateway Ref Fake GW", "authz-gateway-ref-fakegw", exampleGw, ""},
	{"Authz Policies - GatewayClass Ref", "authz-gatewayclass-ref", exampleGw, ""},
	{"Authz Policies - GatewayClass Ref Non-Root NS", "authz-gatewayclass-ref-nonrootns", exampleGw, ""},
	{"Authz Policies - ServiceEntry", "authz-serviceentry", exampleGw, ""},
	{"Authz Policies - Multi-Service", "authz-multi-service", exampleGw, ""},
	{"Authz Policies - CUSTOM", "authz-custom", exampleGw, ""},
	{"No listeners", "empty", exampleGw, ""},
}

func TestWaypointTranslator(t *testing.T) {
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			gomega.RegisterTestingT(t)

			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			ctx := t.Context()
			dir := fsutils.MustGetThisDir()

			translatortest.TestTranslationWithExtraPlugins(
				t,
				ctx,
				[]string{filepath.Join(dir, "testdata/input", tt.file+".yaml")},
				filepath.Join(dir, "testdata/output", tt.file+".yaml"),
				tt.gw,
				waypointExtraConfig(),
				waypointSettings,
			)
		})
	}
}

// TestWaypointListenerAttachedRoutes proves waypoint Gateways report AttachedRoutes
// for HTTPRoutes collected at Gateway and Service attachment. Waypoint translation
// skips the core translator's setAttachedRoutes, so this must be asserted here.
func TestWaypointListenerAttachedRoutes(t *testing.T) {
	tests := []struct {
		name string
		file string
		want int32
	}{
		{
			name: "HTTPRoute parented to the waypoint Gateway",
			file: "httproute-gateway",
			want: 1,
		},
		{
			name: "HTTPRoute parented to a Service using the waypoint",
			file: "httproute-svc",
			want: 1,
		},
		{
			name: "HTTPRoute parented to a ServiceEntry using the waypoint",
			file: "httproute-se",
			want: 1,
		},
		{
			name: "no HTTPRoutes",
			file: "empty",
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runWaypointTranslation(t, tt.file)
			gw := result.Gateways[exampleGw]
			require.NotNil(t, gw, "gateway fixture must be present")

			status := result.ReportsMap.BuildGWStatus(*gw, nil)
			require.NotNil(t, status, "waypoint translation must produce a Gateway status report")

			attached := proxyListenerAttachedRoutes(status)
			require.Equal(t, tt.want, attached,
				"proxy listener AttachedRoutes should count attached HTTPRoutes")
		})
	}
}

func waypointExtraConfig() translatortest.ExtraConfig {
	return translatortest.ExtraConfig{
		PluginsFn: func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []pluginsdk.Plugin {
			return []pluginsdk.Plugin{waypoint.NewPlugin(ctx, commoncol, wellknown.DefaultWaypointClassName)}
		},
	}
}

func waypointSettings(s *apisettings.Settings) {
	s.EnableExperimentalGatewayAPIFeatures = true
	s.EnableIstioIntegration = true
	s.EnableAuthMetadata = true
}

func runWaypointTranslation(t *testing.T, inputFile string) translatortest.ActualTestResult {
	t.Helper()
	ctx := t.Context()
	dir := fsutils.MustGetThisDir()
	scheme := translatortest.NewScheme(nil)
	results, err := (translatortest.TestCase{
		InputFiles: []string{filepath.Join(dir, "testdata/input", inputFile+".yaml")},
	}).Run(t, ctx, scheme, waypointExtraConfig(), waypointSettings)
	require.NoError(t, err, "waypoint translation should succeed")
	require.Contains(t, results, exampleGw, "expected waypoint Gateway in translation results")
	return results[exampleGw]
}

func proxyListenerAttachedRoutes(status *gwv1.GatewayStatus) int32 {
	for _, listener := range status.Listeners {
		if listener.Name == "proxy" {
			return listener.AttachedRoutes
		}
	}
	return -1
}

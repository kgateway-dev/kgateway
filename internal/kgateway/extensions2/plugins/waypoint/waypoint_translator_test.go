package waypoint_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
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
	{"No listeners", "empty", exampleGw, ""},
}

func TestWaypointTranslator(t *testing.T) {
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			gomega.RegisterTestingT(t)

			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := fsutils.MustGetThisDir()

			extraPluginsFn := func(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []pluginsdk.Plugin {
				return []pluginsdk.Plugin{waypoint.NewPlugin(ctx, commoncol, wellknown.DefaultWaypointClassName)}
			}
			translatortest.TestTranslationWithExtraPlugins(
				t,
				ctx,
				[]string{filepath.Join(dir, "testdata/input", tt.file+".yaml")},
				filepath.Join(dir, "testdata/output", tt.file+".yaml"),
				tt.gw,
				extraPluginsFn,
				nil, nil, "",
			)
		})
	}
}

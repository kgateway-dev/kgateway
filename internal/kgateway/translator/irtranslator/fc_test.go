package irtranslator_test

import (
	"context"
	"testing"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"

	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"

	"k8s.io/apimachinery/pkg/runtime/schema"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	testPluginFilterName = "filter-from-plugin"
	testCustomFilterName = "filter-from-fc-field"
)

var addFiltersGK = schema.GroupKind{
	Group: "test.kgateway.dev",
	Kind:  "AddFilterForTest",
}

// addFilters implements a test translation pass that adds network filters
type addFilters struct {
	ir.UnimplementedProxyTranslationPass
}

func (a addFilters) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return []plugins.StagedNetworkFilter{
		{
			Filter: &listenerv3.Filter{Name: testPluginFilterName},
			Stage:  plugins.BeforeStage(plugins.AuthZStage),
		},
	}, nil
}

func TestFilterChains(t *testing.T) {
	ctx := context.Background()

	translator := irtranslator.Translator{
		// not used by the test today, but if we refactor to call newPass in the test
		// it will be necessary; leaving it here to save time debugging after a refactor
		ContributedPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			addFiltersGK: {
				NewGatewayTranslationPass: func(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
					return addFilters{}
				},
			},
		},
	}

	// Create test gateway and listener IR
	gateway := ir.GatewayIR{SourceObject: &ir.Gateway{Obj: &gwv1.Gateway{}}}
	listener := ir.ListenerIR{
		HttpFilterChain: []ir.HttpFilterChainIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "httpchain",
				CustomNetworkFilters: []ir.CustomEnvoyFilter{{
					Name:        testCustomFilterName,
					FilterStage: plugins.BeforeStage(plugins.AuthZStage),
				}},
			},
		}},
		TcpFilterChain: []ir.TcpIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "tcpchain",
				CustomNetworkFilters: []ir.CustomEnvoyFilter{{
					Name:        testCustomFilterName,
					FilterStage: plugins.BeforeStage(plugins.AuthZStage),
				}},
			},
		}},
	}

	// fake
	reportMap := reports.NewReportMap()
	reporter := reports.NewReporter(&reportMap)

	// method under test
	envoyListener, _ := translator.ComputeListener(
		ctx,
		irtranslator.TranslationPassPlugins{
			addFiltersGK: &irtranslator.TranslationPass{ProxyTranslationPass: addFilters{}},
		},
		gateway,
		listener,
		reporter,
	)

	expectedChainCount := len(listener.HttpFilterChain) + len(listener.TcpFilterChain)
	if len(envoyListener.FilterChains) != expectedChainCount {
		t.Fatal("got", len(envoyListener.FilterChains), "Envoy filter chains, but wanted", expectedChainCount)
	}

	expectedFilters := []string{testPluginFilterName, testCustomFilterName}
	for _, filterChain := range envoyListener.FilterChains {
		for _, expectedFilterName := range expectedFilters {
			filter := ptr.Flatten(slices.FindFunc(filterChain.Filters, func(filter *listenerv3.Filter) bool {
				return filter.Name == expectedFilterName
			}))

			if filter == nil {
				t.Errorf("filter chain %q missing expected filter %q",
					filterChain.Name, expectedFilterName)
			}
		}
	}
}

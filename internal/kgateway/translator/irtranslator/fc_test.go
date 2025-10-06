package irtranslator_test

import (
	"context"
	"testing"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
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

func (a addFilters) NetworkFilters() ([]plugins.StagedNetworkFilter, error) {
	return []plugins.StagedNetworkFilter{
		{
			Filter: &envoylistenerv3.Filter{Name: testPluginFilterName},
			Stage:  plugins.BeforeStage(plugins.AuthZStage),
		},
	}, nil
}

func TestFilterChains(t *testing.T) {
	ctx := context.Background()

	translator := irtranslator.Translator{
		// not used by the test today, but if we refactor to call newPass in the test
		// it will be necessary; leaving it here to save time debugging after a refactor
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			addFiltersGK: {
				NewGatewayTranslationPass: func(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
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
	assert.Equal(t, expectedChainCount, len(envoyListener.FilterChains), "unexpected number of Envoy filter chains")

	expectedFilters := []string{testPluginFilterName, testCustomFilterName}
	for _, filterChain := range envoyListener.FilterChains {
		for _, expectedFilterName := range expectedFilters {
			filter := ptr.Flatten(slices.FindFunc(filterChain.Filters, func(filter *envoylistenerv3.Filter) bool {
				return filter.Name == expectedFilterName
			}))
			assert.NotNil(t, filter, "filter chain %q missing expected filter %q", filterChain.Name, expectedFilterName)
		}
	}
}

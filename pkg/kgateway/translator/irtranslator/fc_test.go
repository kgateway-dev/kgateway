package irtranslator_test

import (
	"context"
	"testing"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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

func (a addFilters) NetworkFilters() ([]filters.StagedNetworkFilter, error) {
	return []filters.StagedNetworkFilter{
		{
			Filter: &envoylistenerv3.Filter{Name: testPluginFilterName},
			Stage:  filters.BeforeStage(filters.AuthZStage),
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
		BindAddress: "0.0.0.0",
		BindPort:    8080,
		HttpFilterChain: []ir.HttpFilterChainIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "httpchain",
				CustomNetworkFilters: []ir.CustomEnvoyFilter{{
					Name:        testCustomFilterName,
					FilterStage: filters.BeforeStage(filters.AuthZStage),
				}},
			},
		}},
		TcpFilterChain: []ir.TcpIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "tcpchain",
				CustomNetworkFilters: []ir.CustomEnvoyFilter{{
					Name:        testCustomFilterName,
					FilterStage: filters.BeforeStage(filters.AuthZStage),
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
	require.NotNil(t, envoyListener, "expected non-nil listener for valid bind address")

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

func TestFilterChainsIPv6(t *testing.T) {
	ctx := context.Background()

	translator := irtranslator.Translator{}

	gateway := ir.GatewayIR{SourceObject: &ir.Gateway{Obj: &gwv1.Gateway{}}}
	listener := ir.ListenerIR{
		BindAddress: "2001:db8::1",
		BindPort:    8080,
		HttpFilterChain: []ir.HttpFilterChainIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "httpchain",
			},
		}},
	}

	reportMap := reports.NewReportMap()
	reporter := reports.NewReporter(&reportMap)

	envoyListener, _ := translator.ComputeListener(
		ctx,
		irtranslator.TranslationPassPlugins{},
		gateway,
		listener,
		reporter,
	)
	require.NotNil(t, envoyListener, "expected non-nil listener for IPv6 bind address")
	assert.Equal(t, "2001:db8::1", envoyListener.Address.GetSocketAddress().Address, "IPv6 address should be set correctly")
}

func TestFilterChainsIPv4MappedIPv6(t *testing.T) {
	ctx := context.Background()

	translator := irtranslator.Translator{}

	gateway := ir.GatewayIR{SourceObject: &ir.Gateway{Obj: &gwv1.Gateway{}}}
	listener := ir.ListenerIR{
		BindAddress: "::ffff:192.168.1.1",
		BindPort:    8080,
		HttpFilterChain: []ir.HttpFilterChainIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "httpchain",
			},
		}},
	}

	reportMap := reports.NewReportMap()
	reporter := reports.NewReporter(&reportMap)

	envoyListener, _ := translator.ComputeListener(
		ctx,
		irtranslator.TranslationPassPlugins{},
		gateway,
		listener,
		reporter,
	)
	require.NotNil(t, envoyListener, "expected non-nil listener for IPv4-mapped IPv6 bind address")
	assert.Equal(t, "::ffff:192.168.1.1", envoyListener.Address.GetSocketAddress().Address, "IPv4-mapped IPv6 address should be set correctly")
}

func TestFilterChainsInvalidIP(t *testing.T) {
	ctx := context.Background()

	translator := irtranslator.Translator{}

	gateway := ir.GatewayIR{SourceObject: &ir.Gateway{Obj: &gwv1.Gateway{}}}
	listener := ir.ListenerIR{
		BindAddress: "not-an-ip",
		BindPort:    8080,
		HttpFilterChain: []ir.HttpFilterChainIR{{
			FilterChainCommon: ir.FilterChainCommon{
				FilterChainName: "httpchain",
			},
		}},
	}

	reportMap := reports.NewReportMap()
	reporter := reports.NewReporter(&reportMap)

	envoyListener, _ := translator.ComputeListener(
		ctx,
		irtranslator.TranslationPassPlugins{},
		gateway,
		listener,
		reporter,
	)
	assert.Nil(t, envoyListener, "expected nil listener for invalid IP bind address")
}

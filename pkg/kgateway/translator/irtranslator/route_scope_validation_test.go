package irtranslator

import (
	"context"
	"errors"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func routeConfigurationFromBootstrap(t *testing.T, config *envoybootstrapv3.Bootstrap) *envoyroutev3.RouteConfiguration {
	t.Helper()
	hcm := &envoyhcm.HttpConnectionManager{}
	require.NoError(t, config.GetStaticResources().GetListeners()[0].GetFilterChains()[0].GetFilters()[0].GetTypedConfig().UnmarshalTo(hcm))
	return hcm.GetRouteConfig()
}

func scopeTestVirtualHosts() []*ir.VirtualHost {
	return []*ir.VirtualHost{
		{Name: "one", Hostname: "one.example.com", Rules: []ir.HttpRouteRuleMatchIR{testRouteIR(0, "/one", "shared-cluster")}},
		{Name: "two", Hostname: "two.example.com", Rules: []ir.HttpRouteRuleMatchIR{testRouteIR(0, "/two", "shared-cluster")}},
	}
}

func TestFinalRouteConfigurationPreservesScopeSettings(t *testing.T) {
	calls := 0
	var validated *envoyroutev3.RouteConfiguration
	v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
		calls++
		validated = routeConfigurationFromBootstrap(t, b)
		require.Len(t, b.GetStaticResources().GetClusters(), 1, "cluster stubs must be deduplicated across virtual hosts")
		return nil
	}}
	h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
	config, err := utils.MessageToAny(wrapperspb.Bool(true))
	require.NoError(t, err)
	attachRouteConfigPass(h, routeConfigPassFunc{applyContext: func(ctx *ir.RouteConfigContext) {
		ctx.TypedFilterConfig.AddTypedConfig("inherited", wrapperspb.Bool(true))
	}, apply: func(cfg *envoyroutev3.RouteConfiguration) {
		cfg.ResponseHeadersToAdd = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "x-test", Value: "scope"}}}
		for _, vh := range cfg.VirtualHosts {
			vh.RateLimits = []*envoyroutev3.RateLimit{{
				Stage: wrapperspb.UInt32(1),
				Actions: []*envoyroutev3.RateLimit_Action{{ActionSpecifier: &envoyroutev3.RateLimit_Action_GenericKey_{
					GenericKey: &envoyroutev3.RateLimit_Action_GenericKey{DescriptorValue: "limited"},
				}}},
			}}
			vh.TypedPerFilterConfig = map[string]*anypb.Any{"vhost": config}
		}
	}})
	cfg := h.ComputeRouteConfiguration(context.Background(), scopeTestVirtualHosts())
	require.Equal(t, 1, calls, "successful strict validation should batch the complete configuration")
	assert.True(t, proto.Equal(cfg, validated), "validation must see the final virtual hosts, routes, headers and inherited configs")
}

func TestFinalRouteConfigurationIsolatesScopeFailures(t *testing.T) {
	config, err := utils.MessageToAny(wrapperspb.Bool(true))
	require.NoError(t, err)
	for _, tc := range []struct {
		name          string
		mutate        func(*envoyroutev3.RouteConfiguration)
		mutateContext func(*ir.RouteConfigContext)
		reject        func(*envoyroutev3.RouteConfiguration) bool
		wholeConfig   bool
	}{
		{
			name: "virtual host rate limit",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].RateLimits = []*envoyroutev3.RateLimit{{Stage: wrapperspb.UInt32(11)}}
			},
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				for _, vh := range cfg.VirtualHosts {
					if len(vh.RateLimits) > 0 {
						return true
					}
				}
				return false
			},
		},
		{
			name: "virtual host inherited filter",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].TypedPerFilterConfig = map[string]*anypb.Any{"bad-filter": config}
			},
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				for _, vh := range cfg.VirtualHosts {
					if vh.TypedPerFilterConfig["bad-filter"] != nil {
						return true
					}
				}
				return false
			},
		},
		{
			name: "empty virtual host rate limit",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].Routes = nil
				cfg.VirtualHosts[0].RateLimits = []*envoyroutev3.RateLimit{{Stage: wrapperspb.UInt32(11)}}
			},
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				for _, vh := range cfg.VirtualHosts {
					if len(vh.RateLimits) > 0 {
						return true
					}
				}
				return false
			},
		},
		{
			name:   "route configuration inherited filter",
			mutate: func(*envoyroutev3.RouteConfiguration) {},
			mutateContext: func(ctx *ir.RouteConfigContext) {
				ctx.TypedFilterConfig.AddTypedConfig("bad-filter", wrapperspb.Bool(true))
			},
			reject:      func(cfg *envoyroutev3.RouteConfiguration) bool { return cfg.TypedPerFilterConfig["bad-filter"] != nil },
			wholeConfig: true,
		},
		{
			name: "route configuration headers",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.ResponseHeadersToAdd = []*envoycorev3.HeaderValueOption{{Header: &envoycorev3.HeaderValue{Key: "bad-header"}}}
			},
			reject:      func(cfg *envoyroutev3.RouteConfiguration) bool { return len(cfg.ResponseHeadersToAdd) > 0 },
			wholeConfig: true,
		},
		{
			name:   "duplicate domains across virtual hosts",
			mutate: func(cfg *envoyroutev3.RouteConfiguration) { cfg.VirtualHosts[1].Domains = cfg.VirtualHosts[0].Domains },
			reject: func(cfg *envoyroutev3.RouteConfiguration) bool {
				return len(cfg.VirtualHosts) == 2 && cfg.VirtualHosts[0].Domains[0] == cfg.VirtualHosts[1].Domains[0]
			},
			wholeConfig: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
				if tc.reject(routeConfigurationFromBootstrap(t, b)) {
					return errors.New("invalid scope configuration")
				}
				return nil
			}}
			h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)
			h.routeConfigName = "original-route-config"
			attachRouteConfigPass(h, routeConfigPassFunc{applyContext: tc.mutateContext, apply: func(cfg *envoyroutev3.RouteConfiguration) {
				cfg.VirtualHosts[0].Domains = append(cfg.VirtualHosts[0].Domains, "alias.example.com")
				tc.mutate(cfg)
			}})
			inputs := scopeTestVirtualHosts()
			rm := reports.NewReportMap()
			h.reporter = reports.NewReporter(&rm)
			for _, vh := range inputs {
				vh.ParentRef = ir.Listener{Parent: &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: vh.Name}}, Listener: gwv1.Listener{Name: "http"}}
			}
			cfg := h.ComputeRouteConfiguration(context.Background(), inputs)
			for i, vh := range inputs {
				gw := vh.ParentRef.Parent.(*gwv1.Gateway)
				report := rm.Gateway(gw)
				if report == nil {
					require.False(t, i == 0 || tc.wholeConfig, "affected listener must have a report")
					continue
				}
				listener := report.Listener(&vh.ParentRef.Listener).(*reports.ListenerReport)
				condition := meta.FindStatusCondition(listener.Status.Conditions, string(gwv1.ListenerConditionAccepted))
				if i == 0 || tc.wholeConfig {
					require.NotNil(t, condition, "affected listener must report replacement")
					assert.Equal(t, string(reportssdk.ListenerReplacedReason), condition.Reason)
					assert.Equal(t, metav1.ConditionFalse, condition.Status)
				} else if condition != nil {
					assert.NotEqual(t, string(reportssdk.ListenerReplacedReason), condition.Reason, "healthy listener must not report replacement")
				}
			}
			assert.False(t, tc.reject(cfg), "invalid scope settings must not survive fallback")
			assert.Equal(t, h.routeConfigName, cfg.Name, "RDS identity must survive fallback")
			if tc.wholeConfig {
				require.Len(t, cfg.VirtualHosts, 1)
				assert.Equal(t, []string{"*"}, cfg.VirtualHosts[0].Domains)
			} else {
				require.Len(t, cfg.VirtualHosts, 2)
				assert.Equal(t, []string{"one.example.com", "alias.example.com"}, cfg.VirtualHosts[0].Domains)
				assert.Equal(t, "shared-cluster", cfg.VirtualHosts[1].Routes[0].GetRoute().GetCluster(), "healthy virtual host must remain unchanged")
			}
			assert.EqualValues(t, 500, cfg.VirtualHosts[0].Routes[0].GetDirectResponse().GetStatus())
		})
	}
}

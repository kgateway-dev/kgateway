package irtranslator

import (
	"context"
	"errors"
	"slices"
	"testing"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	testFilterChainName = "http-chain"
	testPluginFilter    = "plugin-filter"
	invalidRouteCluster = "invalid-cluster"
)

// hcmPass contributes an HTTP filter and HCM settings, the output that only
// exists once routes have been translated.
type hcmPass struct {
	ir.UnimplementedProxyTranslationPass
	applyHCM func(*envoyhcm.HttpConnectionManager)
}

func (hcmPass) HttpFilters(ir.HttpFiltersContext, ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	return []filters.StagedHttpFilter{{
		Filter: &envoyhcm.HttpFilter{
			Name:       testPluginFilter,
			ConfigType: &envoyhcm.HttpFilter_TypedConfig{TypedConfig: utils.MustMessageToAny(wrapperspb.Bool(true))},
		},
		Stage: filters.DuringStage(filters.AuthZStage),
	}}, nil
}

func (p hcmPass) ApplyHCM(_ *ir.HcmContext, out *envoyhcm.HttpConnectionManager) error {
	if p.applyHCM != nil {
		p.applyHCM(out)
	}
	return nil
}

// hcmFromFilters returns the HCM among a filter chain's network filters.
func hcmFromFilters(t *testing.T, networkFilters []*envoylistenerv3.Filter) *envoyhcm.HttpConnectionManager {
	t.Helper()
	for _, filter := range networkFilters {
		if filter.GetName() == wellknown.HTTPConnectionManager {
			hcm := &envoyhcm.HttpConnectionManager{}
			require.NoError(t, filter.GetTypedConfig().UnmarshalTo(hcm))
			return hcm
		}
	}
	require.Fail(t, "filter chain has no HttpConnectionManager")
	return nil
}

func httpFilterNames(hcm *envoyhcm.HttpConnectionManager) []string {
	var names []string
	for _, filter := range hcm.GetHttpFilters() {
		names = append(names, filter.GetName())
	}
	return names
}

func routesInvalidCluster(rc *envoyroutev3.RouteConfiguration) bool {
	for _, vh := range rc.GetVirtualHosts() {
		for _, route := range vh.GetRoutes() {
			if route.GetRoute().GetCluster() == invalidRouteCluster {
				return true
			}
		}
	}
	return false
}

// validatedConfig is what the mock Envoy was asked to validate.
type validatedConfig struct {
	hcm *envoyhcm.HttpConnectionManager
	rc  *envoyroutev3.RouteConfiguration
}

func (c validatedConfig) hasPluginOutput() bool {
	return slices.Contains(httpFilterNames(c.hcm), testPluginFilter)
}

func TestComputeListenerValidatesFinalHTTPFilterChain(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     apisettings.ValidationMode
		vhosts   int
		cluster  string
		applyHCM func(*envoyhcm.HttpConnectionManager)
		// reject reports whether the mock Envoy rejects a configuration.
		reject func(validatedConfig) bool
		// wantCalls counts Envoy invocations. A valid configuration costs no more
		// than route validation alone: the filters share its once-per-chain call.
		wantCalls             int
		wantFilterChainFailed bool
		wantRouteReplaced     bool
	}{
		{
			name:      "valid single virtual host validates filters with routes in one call",
			mode:      apisettings.ValidationStrict,
			vhosts:    1,
			wantCalls: 1,
		},
		{
			name:      "valid virtual hosts validate filters with shared settings",
			mode:      apisettings.ValidationStrict,
			vhosts:    2,
			wantCalls: 3, // shared settings with the filters, then each virtual host
		},
		{
			name:                  "invalid plugin HTTP filter",
			mode:                  apisettings.ValidationStrict,
			vhosts:                1,
			reject:                validatedConfig.hasPluginOutput,
			wantCalls:             2, // the failed full check, then the filters with the fallback
			wantFilterChainFailed: true,
		},
		{
			name:                  "invalid plugin HTTP filter with shared settings",
			mode:                  apisettings.ValidationStrict,
			vhosts:                2,
			reject:                validatedConfig.hasPluginOutput,
			wantCalls:             2,
			wantFilterChainFailed: true,
		},
		{
			name:   "invalid HCM setting applied after routes",
			mode:   apisettings.ValidationStrict,
			vhosts: 1,
			applyHCM: func(hcm *envoyhcm.HttpConnectionManager) {
				hcm.AccessLog = append(hcm.AccessLog, &envoyaccesslogv3.AccessLog{Name: "bad-access-log"})
			},
			reject:                func(c validatedConfig) bool { return len(c.hcm.GetAccessLog()) > 0 },
			wantCalls:             2,
			wantFilterChainFailed: true,
		},
		{
			name:              "invalid route keeps valid filters",
			mode:              apisettings.ValidationStrict,
			vhosts:            1,
			cluster:           invalidRouteCluster,
			reject:            func(c validatedConfig) bool { return routesInvalidCluster(c.rc) },
			wantCalls:         -1, // route isolation is covered by the route validation tests
			wantRouteReplaced: true,
		},
		{
			name:   "standard mode does not invoke Envoy",
			mode:   apisettings.ValidationStandard,
			vhosts: 1,
			reject: func(validatedConfig) bool { return true },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var validated []validatedConfig
			v := &routeMockValidator{validateFunc: func(_ context.Context, b *envoybootstrapv3.Bootstrap) error {
				hcm := hcmFromFilters(t, b.GetStaticResources().GetListeners()[0].GetFilterChains()[0].GetFilters())
				c := validatedConfig{hcm: hcm, rc: hcm.GetRouteConfig()}
				require.NotNil(t, c.rc, "validation must inline the route configuration")
				validated = append(validated, c)
				if tc.reject != nil && tc.reject(c) {
					return errors.New("invalid filter chain")
				}
				return nil
			}}

			cluster := tc.cluster
			if cluster == "" {
				cluster = "cluster-a"
			}
			parent := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}}
			listenerRef := gwv1.Listener{Name: "http"}
			var vhosts []*ir.VirtualHost
			for _, name := range []string{"one", "two"}[:tc.vhosts] {
				vhosts = append(vhosts, &ir.VirtualHost{
					Name:      name,
					Hostname:  name + ".example.com",
					Rules:     []ir.HttpRouteRuleMatchIR{testRouteIR(0, "/", cluster)},
					ParentRef: ir.Listener{Parent: parent, Listener: listenerRef},
				})
			}
			gk := schema.GroupKind{Group: "test", Kind: "HCMPolicy"}
			listener := ir.ListenerIR{
				Name:        "listener",
				BindAddress: "0.0.0.0",
				BindPort:    8080,
				HttpFilterChain: []ir.HttpFilterChainIR{{
					FilterChainCommon: ir.FilterChainCommon{FilterChainName: testFilterChainName},
					Vhosts:            vhosts,
					AttachedPolicies: ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
						gk: {{GroupKind: gk}},
					}},
				}},
			}
			gateway := ir.GatewayIR{SourceObject: &ir.Gateway{Obj: parent}}
			translator := &Translator{ValidationLevel: tc.mode, Validator: v}
			pass := TranslationPassPlugins{gk: &TranslationPass{ProxyTranslationPass: hcmPass{applyHCM: tc.applyHCM}}}
			rm := reports.NewReportMap()

			out, routes := translator.ComputeListener(t.Context(), pass, gateway, listener, reports.NewReporter(&rm))

			if tc.wantCalls >= 0 {
				assert.Len(t, validated, tc.wantCalls)
			}
			if tc.mode == apisettings.ValidationStrict {
				require.NotEmpty(t, validated)
				assert.True(t, validated[0].hasPluginOutput(), "the first check must carry the final filter chain")
				for _, c := range validated[1:] {
					if c.hasPluginOutput() {
						assert.Len(t, c.rc.GetVirtualHosts(), 1, "filters are rechecked only with the fallback")
					}
				}
			}
			require.NotNil(t, out)
			require.Len(t, out.GetFilterChains(), 1)
			hcm := hcmFromFilters(t, out.GetFilterChains()[0].GetFilters())
			assert.Equal(t, testFilterChainName, hcm.GetRds().GetRouteConfigName(), "emitted HCM must keep RDS")
			require.Len(t, routes, 1)
			rc := routes[0]
			assert.Equal(t, testFilterChainName, rc.GetName(), "RDS identity must survive fallback")

			listenerReport := func() *reports.ListenerReport {
				report := rm.Gateway(parent)
				if report == nil {
					return nil
				}
				return report.Listener(&listenerRef).(*reports.ListenerReport)
			}
			if !tc.wantFilterChainFailed {
				assert.Equal(t, []string{testPluginFilter, wellknown.Router}, httpFilterNames(hcm), "valid filters must be kept")
				require.Len(t, rc.GetVirtualHosts(), tc.vhosts)
				route := rc.GetVirtualHosts()[0].GetRoutes()[0]
				if tc.wantRouteReplaced {
					assert.EqualValues(t, 500, route.GetDirectResponse().GetStatus(), "invalid route must be isolated")
				} else {
					assert.Equal(t, cluster, route.GetRoute().GetCluster())
				}
				if report := listenerReport(); report != nil {
					condition := meta.FindStatusCondition(report.Status.Conditions, string(gwv1.ListenerConditionAccepted))
					if condition != nil {
						assert.NotEqual(t, string(reportssdk.ListenerReplacedReason), condition.Reason)
					}
				}
				return
			}

			assert.Equal(t, []string{wellknown.Router}, httpFilterNames(hcm), "rejected filters must not survive fallback")
			assert.Empty(t, hcm.GetAccessLog(), "rejected HCM settings must not survive fallback")
			require.Len(t, rc.GetVirtualHosts(), 1)
			require.Len(t, rc.GetVirtualHosts()[0].GetRoutes(), 1)
			assert.EqualValues(t, 500, rc.GetVirtualHosts()[0].GetRoutes()[0].GetDirectResponse().GetStatus(),
				"routes must not be served without the filters that were rejected")
			assert.Equal(t, []string{"*"}, rc.GetVirtualHosts()[0].GetDomains())

			report := listenerReport()
			require.NotNil(t, report, "affected listener must have a report")
			condition := meta.FindStatusCondition(report.Status.Conditions, string(gwv1.ListenerConditionAccepted))
			require.NotNil(t, condition, "affected listener must report replacement")
			assert.Equal(t, metav1.ConditionFalse, condition.Status)
			assert.Equal(t, string(reportssdk.ListenerReplacedReason), condition.Reason)
			assert.Contains(t, condition.Message, "invalid filter chain")
		})
	}
}

func TestInlineRouteConfigurationPreservesNetworkFilters(t *testing.T) {
	hcmFilter, err := NewFilterWithTypedConfig(wellknown.HTTPConnectionManager, &envoyhcm.HttpConnectionManager{
		StatPrefix:     "http",
		RouteSpecifier: &envoyhcm.HttpConnectionManager_Rds{Rds: &envoyhcm.Rds{RouteConfigName: "rds"}},
	})
	require.NoError(t, err)
	networkFilters := []*envoylistenerv3.Filter{{Name: "custom-network-filter"}, hcmFilter}

	inlined, err := inlineRouteConfiguration(networkFilters, &envoyroutev3.RouteConfiguration{Name: "rds"})
	require.NoError(t, err)

	require.Len(t, inlined, 2)
	assert.Equal(t, "custom-network-filter", inlined[0].GetName(), "network filters must be validated in order")
	assert.Equal(t, "rds", hcmFromFilters(t, inlined).GetRouteConfig().GetName())
	assert.Equal(t, "rds", hcmFromFilters(t, networkFilters).GetRds().GetRouteConfigName(), "emitted filters must not be mutated")
}

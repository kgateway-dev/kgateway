package irtranslator

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

type routeMockValidator struct {
	validateFunc func(context.Context, *envoybootstrapv3.Bootstrap) error
}

var _ validator.Validator = &routeMockValidator{}

func (m *routeMockValidator) Validate(ctx context.Context, config *envoybootstrapv3.Bootstrap) error {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, config)
	}
	return nil
}

func TestValidateWeightedClusters(t *testing.T) {
	tests := []struct {
		name     string
		clusters []*envoyroutev3.WeightedCluster_ClusterWeight
		wantErr  bool
	}{
		{
			name:     "no clusters",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{},
			wantErr:  false,
		},
		{
			name: "single cluster with weight 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
			},
			wantErr: true,
		},
		{
			name: "single cluster with weight > 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(100),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple clusters all with weight 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
				{
					Weight: wrapperspb.UInt32(0),
				},
			},
			wantErr: true,
		},
		{
			name: "multiple clusters with mixed weights",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
				{
					Weight: wrapperspb.UInt32(100),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple clusters all with weight > 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(50),
				},
				{
					Weight: wrapperspb.UInt32(50),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errs []error
			validateWeightedClusters(tt.clusters, &errs)

			if tt.wantErr {
				assert.Len(t, errs, 1)
				assert.Contains(t, errs[0].Error(), "All backend weights are 0. At least one backendRef in the HTTPRoute rule must specify a non-zero weight")
			} else {
				assert.Len(t, errs, 0)
			}
		})
	}
}

func TestSetEnvoyPathMatcher_PathPrefix(t *testing.T) {
	pathPrefix := gwv1.PathMatchPathPrefix

	tests := []struct {
		name         string
		path         string
		wantPrefix   string
		wantSeparate bool
	}{
		{
			name:         "uses path separated prefix for clean prefix",
			path:         "/foo",
			wantPrefix:   "/foo",
			wantSeparate: true,
		},
		{
			name:         "ignores trailing slash for non root prefix",
			path:         "/foo/",
			wantPrefix:   "/foo",
			wantSeparate: true,
		},
		{
			name:         "keeps root prefix unchanged",
			path:         "/",
			wantPrefix:   "/",
			wantSeparate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &envoyroutev3.RouteMatch{}

			setEnvoyPathMatcher(gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathPrefix,
					Value: &tt.path,
				},
			}, out)

			if tt.wantSeparate {
				spec, ok := out.PathSpecifier.(*envoyroutev3.RouteMatch_PathSeparatedPrefix)
				assert.True(t, ok)
				assert.Equal(t, tt.wantPrefix, spec.PathSeparatedPrefix)
				return
			}

			spec, ok := out.PathSpecifier.(*envoyroutev3.RouteMatch_Prefix)
			assert.True(t, ok)
			assert.Equal(t, tt.wantPrefix, spec.Prefix)
		})
	}
}

func TestValidateRouteStrictSkipsMatcherOnlyEnvoyValidationForCommonMatchers(t *testing.T) {
	pathPrefix := gwv1.PathMatchPathPrefix
	pathExact := gwv1.PathMatchExact

	tests := []struct {
		name  string
		match gwv1.HTTPRouteMatch
	}{
		{
			name: "prefix",
			match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathPrefix,
					Value: new("/"),
				},
			},
		},
		{
			name: "exact",
			match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathExact,
					Value: new("/exact"),
				},
			},
		},
		{
			name: "path separated prefix",
			match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathPrefix,
					Value: new("/separated"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			v := &routeMockValidator{validateFunc: func(context.Context, *envoybootstrapv3.Bootstrap) error {
				calls++
				return nil
			}}

			err := validateRoute(context.Background(), testRouteWithMatch(translateMatcher(tt.match)), v, apisettings.ValidationStrict)

			require.NoError(t, err)
			assert.Equal(t, 1, calls, "strict validation should only run full-route Envoy validation")
		})
	}
}

func TestValidateRouteStrictInvalidGeneratedRegexMatcher(t *testing.T) {
	pathRegex := gwv1.PathMatchRegularExpression
	headerRegex := gwv1.HeaderMatchRegularExpression
	queryRegex := gwv1.QueryParamMatchRegularExpression

	tests := []struct {
		name  string
		match gwv1.HTTPRouteMatch
	}{
		{
			name: "path regex",
			match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathRegex,
					Value: new("[[invalid"),
				},
			},
		},
		{
			name: "header regex",
			match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathPrefixPtr,
					Value: new("/"),
				},
				Headers: []gwv1.HTTPHeaderMatch{{
					Type:  &headerRegex,
					Name:  "x-test",
					Value: "[[invalid",
				}},
			},
		},
		{
			name: "query regex",
			match: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathPrefixPtr,
					Value: new("/"),
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{{
					Type:  &queryRegex,
					Name:  "q",
					Value: "[[invalid",
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			v := &routeMockValidator{validateFunc: func(context.Context, *envoybootstrapv3.Bootstrap) error {
				calls++
				return nil
			}}

			err := validateRoute(context.Background(), testRouteWithMatch(translateMatcher(tt.match)), v, apisettings.ValidationStrict)

			require.ErrorIs(t, err, ErrInvalidMatcher)
			assert.Equal(t, 0, calls, "invalid generated matcher should be rejected before Envoy validation")
		})
	}
}

func TestComputeVirtualHostStrictBatchesFullRouteValidation(t *testing.T) {
	calls := 0
	var routeCounts []int
	v := &routeMockValidator{validateFunc: func(_ context.Context, config *envoybootstrapv3.Bootstrap) error {
		calls++
		routeCounts = append(routeCounts, len(routesFromValidationBootstrap(t, config)))
		return nil
	}}
	h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)

	out := h.computeVirtualHost(context.Background(), &ir.VirtualHost{
		Name:     "test-vhost",
		Hostname: "example.com",
		Rules: []ir.HttpRouteRuleMatchIR{
			testRouteIR(0, "/one", "cluster-one"),
			testRouteIR(1, "/two", "cluster-two"),
		},
	})

	require.Len(t, out.GetRoutes(), 2)
	assert.Equal(t, 1, calls)
	assert.Equal(t, []int{2}, routeCounts)
}

func TestComputeVirtualHostStrictIsolatesInvalidRouteAfterBatchFailure(t *testing.T) {
	calls := 0
	var routeCounts []int
	v := &routeMockValidator{validateFunc: func(_ context.Context, config *envoybootstrapv3.Bootstrap) error {
		calls++
		routes := routesFromValidationBootstrap(t, config)
		routeCounts = append(routeCounts, len(routes))
		switch calls {
		case 1:
			require.Len(t, routes, 2)
			return errors.New("batch failed")
		case 2:
			require.Len(t, routes, 1)
			assert.Contains(t, routes[0].GetName(), "route-0")
			return nil
		case 3:
			require.Len(t, routes, 1)
			assert.Contains(t, routes[0].GetName(), "route-1")
			return errors.New("bad route")
		case 4:
			require.Len(t, routes, 1)
			assert.Contains(t, routes[0].GetName(), "route-1")
			return nil
		case 5:
			require.Len(t, routes, 2)
			return nil
		default:
			t.Fatalf("unexpected validation call %d", calls)
			return nil
		}
	}}
	h := testHTTPRouteTranslator(v, apisettings.ValidationStrict)

	out := h.computeVirtualHost(context.Background(), &ir.VirtualHost{
		Name:     "test-vhost",
		Hostname: "example.com",
		Rules: []ir.HttpRouteRuleMatchIR{
			testRouteIR(0, "/one", "cluster-one"),
			testRouteIR(1, "/two", "cluster-two"),
		},
	})

	require.Len(t, out.GetRoutes(), 2)
	_, ok := out.GetRoutes()[0].GetAction().(*envoyroutev3.Route_Route)
	assert.True(t, ok)
	_, ok = out.GetRoutes()[1].GetAction().(*envoyroutev3.Route_DirectResponse)
	assert.True(t, ok)
	assert.Equal(t, []int{2, 1, 1, 1, 2}, routeCounts)
}

func refFor(name string) *ir.AttachedPolicyRef {
	return &ir.AttachedPolicyRef{
		Group:     "gateway.kgateway.dev",
		Kind:      "TrafficPolicy",
		Namespace: "ns",
		Name:      name,
	}
}

func testRouteWithMatch(match *envoyroutev3.RouteMatch) *envoyroutev3.Route {
	return &envoyroutev3.Route{
		Name:  "test-route",
		Match: match,
		Action: &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{
					Cluster: "test-cluster",
				},
			},
		},
	}
}

var pathPrefixPtr = gwv1.PathMatchPathPrefix

func testHTTPRouteTranslator(v validator.Validator, mode apisettings.ValidationMode) *httpRouteConfigurationTranslator {
	return &httpRouteConfigurationTranslator{
		gw: ir.GatewayIR{
			SourceObject: &ir.Gateway{Obj: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{GatewayClassName: "test-class"},
			}},
		},
		pluginPass:      TranslationPassPlugins{},
		logger:          slog.Default(),
		validator:       v,
		validationLevel: mode,
	}
}

func testRouteIR(matchIndex int, path string, clusterName string) ir.HttpRouteRuleMatchIR {
	return ir.HttpRouteRuleMatchIR{
		MatchIndex: matchIndex,
		Match: gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{
				Type:  &pathPrefixPtr,
				Value: new(path),
			},
		},
		Backends: []ir.HttpBackend{{
			Backend: ir.BackendRefIR{
				ClusterName: clusterName,
				Weight:      1,
			},
		}},
	}
}

func routesFromValidationBootstrap(t *testing.T, config *envoybootstrapv3.Bootstrap) []*envoyroutev3.Route {
	t.Helper()
	listeners := config.GetStaticResources().GetListeners()
	require.Len(t, listeners, 1)
	filterChains := listeners[0].GetFilterChains()
	require.Len(t, filterChains, 1)
	filters := filterChains[0].GetFilters()
	require.NotEmpty(t, filters)
	hcm := &envoy_hcm.HttpConnectionManager{}
	require.NoError(t, filters[0].GetTypedConfig().UnmarshalTo(hcm))
	vhosts := hcm.GetRouteConfig().GetVirtualHosts()
	require.Len(t, vhosts, 1)
	return vhosts[0].GetRoutes()
}

func TestSummarizeRuleErrors_NilReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", summarizeRuleErrors(nil))
}

func TestSummarizeRuleErrors_BareErrorPassesThrough(t *testing.T) {
	got := summarizeRuleErrors(errors.New("plain"))
	assert.Equal(t, "plain", got)
}

func TestSummarizeRuleErrors_AttributedAndSorted(t *testing.T) {
	// Insert in reverse-alphabetical order to verify the formatter sorts.
	errs := []error{
		&ir.PolicyError{Ref: refFor("z-pol"), Err: errors.New("z msg")},
		&ir.PolicyError{Ref: refFor("a-pol"), Err: errors.New("a msg")},
	}
	got := summarizeRuleErrors(errors.Join(errs...))
	want := "gateway.kgateway.dev/TrafficPolicy/ns/a-pol: a msg\n" +
		"gateway.kgateway.dev/TrafficPolicy/ns/z-pol: z msg"
	assert.Equal(t, want, got)
}

func TestSummarizeRuleErrors_DedupesIdenticalEntries(t *testing.T) {
	r := refFor("p")
	errs := []error{
		&ir.PolicyError{Ref: r, Err: errors.New("dup")},
		&ir.PolicyError{Ref: r, Err: errors.New("dup")},
		&ir.PolicyError{Ref: r, Err: errors.New("unique")},
	}
	got := summarizeRuleErrors(errors.Join(errs...))
	want := "gateway.kgateway.dev/TrafficPolicy/ns/p: dup\n" +
		"gateway.kgateway.dev/TrafficPolicy/ns/p: unique"
	assert.Equal(t, want, got)
}

func TestSummarizeRuleErrors_MixedAttributedAndBare(t *testing.T) {
	errs := []error{
		&ir.PolicyError{Ref: refFor("p"), Err: errors.New("attributed")},
		errors.New("bare"),
	}
	got := summarizeRuleErrors(errors.Join(errs...))
	// Bare entry sorts first because its refID is the empty string.
	want := "bare\n" +
		"gateway.kgateway.dev/TrafficPolicy/ns/p: attributed"
	assert.Equal(t, want, got)
}

func TestSummarizeRuleErrors_DistinguishesBySection(t *testing.T) {
	// Same policy ref but two different SectionName values producing the same
	// underlying error must NOT be deduped — they correspond to distinct
	// attachments (e.g. two different Gateway listeners).
	mkRef := func(section string) *ir.AttachedPolicyRef {
		return &ir.AttachedPolicyRef{
			Group:       "gateway.kgateway.dev",
			Kind:        "TrafficPolicy",
			Namespace:   "ns",
			Name:        "p",
			SectionName: section,
		}
	}
	errs := []error{
		&ir.PolicyError{Ref: mkRef("http-b"), Err: errors.New("ext not found")},
		&ir.PolicyError{Ref: mkRef("http-a"), Err: errors.New("ext not found")},
	}
	got := summarizeRuleErrors(errors.Join(errs...))
	want := "gateway.kgateway.dev/TrafficPolicy/ns/p/http-a: ext not found\n" +
		"gateway.kgateway.dev/TrafficPolicy/ns/p/http-b: ext not found"
	assert.Equal(t, want, got)
}

func TestSummarizeRuleErrors_FlattensNestedJoins(t *testing.T) {
	inner := errors.Join(
		&ir.PolicyError{Ref: refFor("a-pol"), Err: errors.New("a")},
		&ir.PolicyError{Ref: refFor("b-pol"), Err: errors.New("b")},
	)
	outer := errors.Join(inner, &ir.PolicyError{Ref: refFor("c-pol"), Err: errors.New("c")})
	got := summarizeRuleErrors(outer)
	want := "gateway.kgateway.dev/TrafficPolicy/ns/a-pol: a\n" +
		"gateway.kgateway.dev/TrafficPolicy/ns/b-pol: b\n" +
		"gateway.kgateway.dev/TrafficPolicy/ns/c-pol: c"
	assert.Equal(t, want, got)
}

package httproute

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TranslateGatewayHTTPRouteRules(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	route gwv1.HTTPRoute,
	reporter reports.ParentRefReporter,
) []*routeutils.SortableRoute {
	var outputSortableRoutes []*routeutils.SortableRoute
	for i, rule := range route.Spec.Rules {
		outputRoutes := translateGatewayHTTPRouteRule(
			ctx,
			plugins,
			queries,
			&route,
			rule,
			reporter,
		)
		for j, outputRoute := range outputRoutes {
			sr := &routeutils.SortableRoute{
				InputMatch: rule.Matches[j],
				Route:      outputRoute,
				HttpRoute:  &route,
				Idx:        i,
			}

			outputSortableRoutes = append(outputSortableRoutes, sr)
		}
	}
	return outputSortableRoutes
}

func translateGatewayHTTPRouteRule(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	queries query.GatewayQueries,
	gwroute *gwv1.HTTPRoute,
	rule gwv1.HTTPRouteRule,
	reporter reports.ParentRefReporter,
) []*routev3.Route {
	baseOutputRoute := &routev3.Route{}
	if len(rule.BackendRefs) > 0 {
		baseOutputRoute.Action = translateRouteAction(
			queries,
			gwroute,
			rule.BackendRefs,
			reporter,
		)
	}
	if err := applyFilters(
		ctx,
		plugins,
		rule.Filters,
		baseOutputRoute,
	); err != nil {
		reporter.SetCondition(reports.HTTPRouteCondition{
			Type:   gwv1.RouteConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: gwv1.RouteReasonIncompatibleFilters,
		})
		// drop route
		return nil
	}
	if baseOutputRoute.Action == nil {
		// TODO: maybe? report error
		baseOutputRoute.Action = &routev3.Route_DirectResponse{
			DirectResponse: &routev3.DirectResponseAction{
				Status: http.StatusInternalServerError,
			},
		}
	}

	routes := make([]*routev3.Route, 0, len(rule.Matches))
	for _, match := range rule.Matches {
		outputRoute := proto.Clone(baseOutputRoute).(*routev3.Route)
		outputRoute.Match = translateGlooMatcher(match)
		routes = append(routes, outputRoute)
	}
	return routes
}

func translateGlooMatcher(matcher gwv1.HTTPRouteMatch) *routev3.RouteMatch {
	match := &routev3.RouteMatch{
		Headers:         envoyHeaderMatcher(matcher.Headers),
		QueryParameters: envoyQueryMatcher(matcher.QueryParams),
	}
	if matcher.Method != nil {
		match.Headers = append(match.GetHeaders(), &routev3.HeaderMatcher{
			Name: ":method",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{
				ExactMatch: string(*matcher.Method),
			},
		})
	}
	// need to do this because Go's proto implementation makes oneofs private
	// which genius thought of that?
	setEnvoyPathMatcher(matcher, match)
	return match
}

var separatedPathRegex = regexp.MustCompile("^[^?#]+[^?#/]$")

func setEnvoyPathMatcher(match gwv1.HTTPRouteMatch, out *routev3.RouteMatch) {
	pathType, pathValue := routeutils.ParsePath(match.Path)
	switch pathType {
	case gwv1.PathMatchPathPrefix:
		if !separatedPathRegex.MatchString(pathValue) {
			out.PathSpecifier = &routev3.RouteMatch_Prefix{
				Prefix: pathValue,
			}
		} else {
			out.PathSpecifier = &routev3.RouteMatch_PathSeparatedPrefix{
				PathSeparatedPrefix: pathValue,
			}
		}
	case gwv1.PathMatchExact:
		out.PathSpecifier = &routev3.RouteMatch_Path{
			Path: pathValue,
		}
	case gwv1.PathMatchRegularExpression:
		out.PathSpecifier = &routev3.RouteMatch_SafeRegex{
			SafeRegex: regexutils.NewRegexWithProgramSize(pathValue, nil),
		}
	}
}

func envoyHeaderMatcher(in []gwv1.HTTPHeaderMatch) []*routev3.HeaderMatcher {
	var out []*routev3.HeaderMatcher
	for _, matcher := range in {

		envoyMatch := &routev3.HeaderMatcher{
			Name: string(matcher.Name),
		}
		regex := false
		if matcher.Type != nil && *matcher.Type == gwv1.HeaderMatchRegularExpression {
			regex = true
		}

		// TODO: not sure if we should do PresentMatch according to the spec.
		if matcher.Value == "" {
			envoyMatch.HeaderMatchSpecifier = &routev3.HeaderMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if regex {
				envoyMatch.HeaderMatchSpecifier = &routev3.HeaderMatcher_SafeRegexMatch{
					SafeRegexMatch: regexutils.NewRegexWithProgramSize(matcher.Value, nil),
				}
			} else {
				envoyMatch.HeaderMatchSpecifier = &routev3.HeaderMatcher_ExactMatch{
					ExactMatch: matcher.Value,
				}
			}
		}
		out = append(out, envoyMatch)
	}
	return out
}

func envoyQueryMatcher(in []gwv1.HTTPQueryParamMatch) []*routev3.QueryParameterMatcher {
	var out []*routev3.QueryParameterMatcher
	for _, matcher := range in {
		envoyMatch := &routev3.QueryParameterMatcher{
			Name: string(matcher.Name),
		}
		regex := false
		if matcher.Type != nil && *matcher.Type == gwv1.QueryParamMatchRegularExpression {
			regex = true
		}

		// TODO: not sure if we should do PresentMatch according to the spec.
		if matcher.Value == "" {
			envoyMatch.QueryParameterMatchSpecifier = &routev3.QueryParameterMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if regex {
				envoyMatch.QueryParameterMatchSpecifier = &routev3.QueryParameterMatcher_StringMatch{
					StringMatch: &envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: regexutils.NewRegexWithProgramSize(matcher.Value, nil),
						},
					},
				}
			} else {
				envoyMatch.QueryParameterMatchSpecifier = &routev3.QueryParameterMatcher_StringMatch{
					StringMatch: &envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
							Exact: matcher.Value,
						},
					},
				}
			}
		}
		out = append(out, envoyMatch)
	}
	return out
}

func translateRouteAction(
	queries query.GatewayQueries,
	gwroute *gwv1.HTTPRoute,
	backendRefs []gwv1.HTTPBackendRef,
	reporter reports.ParentRefReporter,
) *routev3.Route_Route {
	var clusters []*routev3.WeightedCluster_ClusterWeight

	for _, backendRef := range backendRefs {
		clusterName := "blackhole_cluster"
		cli, err := queries.GetBackendForRef(context.TODO(), queries.ObjToFrom(gwroute), &backendRef)
		if err != nil {
			switch {
			case errors.Is(err, query.ErrUnknownKind):
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonInvalidKind,
				})
			case errors.Is(err, query.ErrMissingReferenceGrant):
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonRefNotPermitted,
				})
			case apierrors.IsNotFound(err):
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})
			default:
				// setting other errors to not found. not sure if there's a better option.
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})
			}
		} else {
			var port uint32
			if backendRef.Port != nil {
				port = uint32(*backendRef.Port)
			}
			switch cli := cli.(type) {
			case *corev1.Service:
				if port == 0 {
					reporter.SetCondition(reports.HTTPRouteCondition{
						Type:   gwv1.RouteConditionResolvedRefs,
						Status: metav1.ConditionFalse,
						Reason: gwv1.RouteReasonUnsupportedValue,
					})
				} else {
					clusterName = kubernetes.UpstreamName(cli.Namespace, cli.Name, int32(port))
				}
			default:
				reporter.SetCondition(reports.HTTPRouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonInvalidKind,
				})
			}
		}

		var weight *wrappers.UInt32Value
		if backendRef.Weight != nil {
			weight = &wrappers.UInt32Value{
				Value: uint32(*backendRef.Weight),
			}
		} else {
			// according to spec, default weight is 1
			weight = &wrappers.UInt32Value{
				Value: 1,
			}
		}

		// get backend for ref - we must do it to make sure we have permissions to access it.
		// also we need the service so we can translate its name correctly.

		clusters = append(clusters, &routev3.WeightedCluster_ClusterWeight{
			Name:   clusterName,
			Weight: weight,
		})
	}

	action := &routev3.RouteAction{
		ClusterNotFoundResponseCode: routev3.RouteAction_INTERNAL_SERVER_ERROR,
	}
	routeAction := &routev3.Route_Route{
		Route: action,
	}
	switch len(clusters) {
	// case 0:
	//TODO: we should never get here
	case 1:
		action.ClusterSpecifier = &routev3.RouteAction_Cluster{
			Cluster: clusters[0].Name,
		}

	default:
		action.ClusterSpecifier = &routev3.RouteAction_WeightedClusters{
			WeightedClusters: &routev3.WeightedCluster{
				Clusters: clusters,
			},
		}
	}
	return routeAction
}

func applyFilters(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	filters []gwv1.HTTPRouteFilter,
	outputRoute *routev3.Route,
) error {
	for _, filter := range filters {
		if err := applyFilterPlugin(ctx, plugins, filter, outputRoute); err != nil {
			return err
		}
	}
	return nil
}

func applyFilterPlugin(
	ctx context.Context,
	plugins registry.HTTPFilterPluginRegistry,
	filter gwv1.HTTPRouteFilter,
	outputRoute *routev3.Route,
) error {
	var (
		plugin filterplugins.FilterPlugin
		err    error
	)
	if filter.Type == gwv1.HTTPRouteFilterExtensionRef {
		plugin, err = plugins.GetExtensionPlugin(filter.ExtensionRef)
	} else {
		plugin, err = plugins.GetStandardPlugin(filter.Type)
	}
	if err != nil {
		return err
	}

	return plugin.ApplyFilter(
		ctx,
		filter,
		outputRoute,
	)
}

package ir

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	"github.com/solo-io/gloo/projects/controller/pkg/utils"
	extensions "github.com/solo-io/gloo/projects/gateway2/extensions2"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type httpRouteConfigurationTranslator struct {
	gw       model.GatewayIR
	listener model.ListenerIR

	parentRef                gwv1.ParentReference
	routeConfigName          string
	reporter                 reports.Reporter
	requireTlsOnVirtualHosts bool
	PluginPass               map[schema.GroupKind]extensions.ProxyTranslationPass
}

func (h *httpRouteConfigurationTranslator) ComputeRouteConfiguration(ctx context.Context, vhosts []*model.VirtualHost) []*envoy_config_route_v3.RouteConfiguration {
	ctx = contextutils.WithLogger(ctx, "compute_route_config."+h.routeConfigName)
	cfg := &envoy_config_route_v3.RouteConfiguration{
		Name:         h.routeConfigName,
		VirtualHosts: h.computeVirtualHosts(ctx, vhosts),
		//		MaxDirectResponseBodySizeBytes: h.parentListener.GetRouteOptions().GetMaxDirectResponseBodySizeBytes(),
	}
	// Gateway API spec requires that port values in HTTP Host headers be ignored when performing a match
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteSpec - hostnames field
	cfg.IgnorePortInHostMatching = true

	//	if mostSpecificVal := h.parentListener.GetRouteOptions().GetMostSpecificHeaderMutationsWins(); mostSpecificVal != nil {
	//		cfg.MostSpecificHeaderMutationsWins = mostSpecificVal.GetValue()
	//	}

	return []*envoy_config_route_v3.RouteConfiguration{cfg}
}

func (h *httpRouteConfigurationTranslator) computeVirtualHosts(ctx context.Context, virtualHosts []*model.VirtualHost) []*envoy_config_route_v3.VirtualHost {

	var envoyVirtualHosts []*envoy_config_route_v3.VirtualHost
	for _, virtualHost := range virtualHosts {

		envoyVirtualHosts = append(envoyVirtualHosts, h.computeVirtualHost(ctx, virtualHost))
	}
	return envoyVirtualHosts
}

func (h *httpRouteConfigurationTranslator) computeVirtualHost(
	ctx context.Context,
	virtualHost *model.VirtualHost,
) *envoy_config_route_v3.VirtualHost {
	sanitizedName := utils.SanitizeForEnvoy(ctx, virtualHost.Name, "virtual host")

	var envoyRoutes []*envoy_config_route_v3.Route
	for i, route := range virtualHost.Rules {
		routeReport := h.reporter.Route(route.Parent.SourceObject).ParentRef(&h.parentRef)
		generatedName := fmt.Sprintf("%s-route-%d", virtualHost.Name, i)
		computedRoutes := h.envoyRoutes(ctx, virtualHost, routeReport, route, generatedName)
		envoyRoutes = append(envoyRoutes, computedRoutes...)
	}
	domains := virtualHost.Hostnames
	if len(domains) == 0 || (len(domains) == 1 && domains[0] == "") {
		domains = []string{"*"}
	}
	var envoyRequireTls envoy_config_route_v3.VirtualHost_TlsRequirementType
	if h.requireTlsOnVirtualHosts {
		// TODO (ilackarms): support external-only TLS
		envoyRequireTls = envoy_config_route_v3.VirtualHost_ALL
	}

	out := &envoy_config_route_v3.VirtualHost{
		Name:       sanitizedName,
		Domains:    domains,
		Routes:     envoyRoutes,
		RequireTls: envoyRequireTls,
	}

	// run the http plugins that are attached to the listener or gateway on the virtual host
	h.runVostPlugins(ctx, out)
	for gvk, pols := range h.listener.AttachedPolicies.Policies {
		pass := h.PluginPass[gvk]
		if pass == nil {
			// TODO: should never happen, log error and report condition
			continue
		}
		for _, pol := range pols {
			pctx := &extensions.VirtualHostContext{
				Policy: pol.Obj(),
			}
			pass.ApplyVhostPlugin(ctx, pctx, out)
			// TODO: check return value, if error returned, log error and report condition
		}
	}

	return out
}

func (h *httpRouteConfigurationTranslator) envoyRoutes(ctx context.Context,
	virtualHost *model.VirtualHost,
	routeReport reports.ParentRefReporter,
	in model.HttpRouteRuleIR,
	generatedName string,
) []*envoy_config_route_v3.Route {

	out := h.initRoutes(virtualHost, in, routeReport, generatedName)

	for i := range out {
		if len(in.BackendRefs) > 0 {
			out[i].Action = translateRouteAction(in)
		}
		// run plugins here that may set actoin
		err := h.runRoutePlugins(ctx, routeReport, in, out[i])

		if err == nil {
			err = validateEnvoyRoute(out[i])
		}
		if err != nil {
			contextutils.LoggerFrom(ctx).Desugar().Debug("invalid route", zap.Error(err))
			// TODO: we may want to aggregate all these errors per http route object and report one message?
			routeReport.SetCondition(reports.RouteCondition{
				Type:   gwv1.RouteConditionPartiallyInvalid,
				Status: metav1.ConditionTrue,
				Reason: gwv1.RouteConditionReason(err.Error()),
				// The message for this condition MUST start with the prefix "Dropped Rule"
				Message: fmt.Sprintf("Dropped Rule: %v", err),
			})
			out[i] = nil
		}
	}

	return slices.DeleteFunc(out, func(e *envoy_config_route_v3.Route) bool { return e == nil })
}

func (h *httpRouteConfigurationTranslator) runVostPlugins(ctx context.Context, out *envoy_config_route_v3.VirtualHost) {
	attachedPoliciesSlice := []model.AttachedPolicies[model.HttpPolicy]{
		h.gw.AttachedHttpPolicies,
		h.listener.AttachedPolicies,
	}
	for _, attachedPolicies := range attachedPoliciesSlice {
		for gk, pols := range attachedPolicies.Policies {
			pass := h.PluginPass[gk]
			if pass == nil {
				// TODO: should never happen, log error and report condition
				continue
			}
			for _, pol := range pols {
				pctx := &extensions.VirtualHostContext{
					Policy: pol.Obj(),
				}
				pass.ApplyVhostPlugin(ctx, pctx, out)
				// TODO: check return value, if error returned, log error and report condition
			}
		}
	}
}

func (h *httpRouteConfigurationTranslator) runRoutePlugins(ctx context.Context, routeReport reports.ParentRefReporter, in model.HttpRouteRuleIR, out *envoy_config_route_v3.Route) error {

	// all policies up to listener have been applied as vhost polices; we need to apply the httproute policies and below

	attachedPoliciesSlice := []model.AttachedPolicies[model.HttpPolicy]{
		in.Parent.AttachedPolicies,
		in.AttachedPolicies,
		in.ExtensionRefs,
	}

	var errs []error

	for _, attachedPolicies := range attachedPoliciesSlice {
		for gk, pols := range attachedPolicies.Policies {
			pass := h.PluginPass[gk]
			if pass == nil {
				// TODO: should never happen, log error and report condition
				continue
			}
			for _, pol := range pols {
				pctx := &extensions.RouteContext{
					Policy:   pol.Obj(),
					Reporter: routeReport,
				}
				err := pass.ApplyForRoute(ctx, pctx, out)
				if err != nil {
					errs = append(errs, err)
				}
				// TODO: check return value, if error returned, log error and report condition
			}
		}
	}
	return errors.Join(errs...)
}

func translateRouteAction(
	in model.HttpRouteRuleIR,
) *envoy_config_route_v3.Route_Route {
	var clusters []*envoy_config_route_v3.WeightedCluster_ClusterWeight

	for _, backend := range in.Backends {
		clusterName := backend.ClusterName

		var weight *wrapperspb.UInt32Value
		if backend.Weight != 0 {
			weight = wrapperspb.UInt32(backend.Weight)
		} else {
			// according to spec, default weight is 1
			weight = wrapperspb.UInt32(1)
		}

		// get backend for ref - we must do it to make sure we have permissions to access it.
		// also we need the service so we can translate its name correctly.

		clusters = append(clusters, &envoy_config_route_v3.WeightedCluster_ClusterWeight{
			Name:   clusterName,
			Weight: weight,
		})
	}

	action := &envoy_config_route_v3.RouteAction{
		ClusterNotFoundResponseCode: envoy_config_route_v3.RouteAction_INTERNAL_SERVER_ERROR,
	}
	routeAction := &envoy_config_route_v3.Route_Route{
		Route: action,
	}
	switch len(clusters) {
	// case 0:
	//TODO: we should never get here
	case 1:
		action.ClusterSpecifier = &envoy_config_route_v3.RouteAction_Cluster{
			Cluster: clusters[0].GetName(),
		}

	default:
		action.ClusterSpecifier = &envoy_config_route_v3.RouteAction_WeightedClusters{
			WeightedClusters: &envoy_config_route_v3.WeightedCluster{
				Clusters: clusters,
			},
		}
	}
	return routeAction
}

func validateEnvoyRoute(r *envoy_config_route_v3.Route) error {
	var errs []error
	match := r.GetMatch()
	route := r.GetRoute()
	re := r.GetRedirect()
	validatePath(match.GetPath(), &errs)
	validatePath(match.GetPrefix(), &errs)
	validatePath(match.GetPathSeparatedPrefix(), &errs)
	validatePath(re.GetPathRedirect(), &errs)
	validatePath(re.GetHostRedirect(), &errs)
	validatePath(re.GetSchemeRedirect(), &errs)
	validatePrefixRewrite(route.GetPrefixRewrite(), &errs)
	validatePrefixRewrite(re.GetPrefixRewrite(), &errs)

	// if the only error is that we have no action, we don't need to drop the route,

	if len(errs) == 0 {
		if r.GetAction() == nil {
			// TODO: maybe? report error
			// r.Action = &envoy_config_route_v3.Route_DirectResponse{
			// 	DirectResponse: &envoy_config_route_v3.DirectResponseAction{
			// 		Status: http.StatusInternalServerError,
			// 	},
			// }
			errs = append(errs, errors.New("no action specified"))
		}
	}
	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("error %s: %w", r.Name, errors.Join(errs...))
}

// creates Envoy routes for each matcher provided on our Gateway route
func (h *httpRouteConfigurationTranslator) initRoutes(
	virtualHost *model.VirtualHost,
	in model.HttpRouteRuleIR,
	routeReport reports.ParentRefReporter,
	generatedName string,
) []*envoy_config_route_v3.Route {

	if len(in.Matches) == 0 {
		return []*envoy_config_route_v3.Route{
			{
				Match: &envoy_config_route_v3.RouteMatch{
					PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{Prefix: "/"},
				},
			},
		}
	}

	out := make([]*envoy_config_route_v3.Route, len(in.Matches))
	for i, matcher := range in.Matches {

		out[i] = &envoy_config_route_v3.Route{
			Match: translateGlooMatcher(matcher),
		}
		name := defaultStr(in.Name)
		if name != "" {
			out[i].Name = fmt.Sprintf("%s-%s-matcher-%d", generatedName, name, i)
		} else {
			out[i].Name = fmt.Sprintf("%s-matcher-%d", generatedName, i)
		}
	}

	return out
}

func defaultStr[T ~string](s *T) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func translateGlooMatcher(matcher gwv1.HTTPRouteMatch) *envoy_config_route_v3.RouteMatch {
	match := &envoy_config_route_v3.RouteMatch{
		Headers:         envoyHeaderMatcher(matcher.Headers),
		QueryParameters: envoyQueryMatcher(matcher.QueryParams),
	}
	if matcher.Method != nil {
		match.Headers = append(match.GetHeaders(), &envoy_config_route_v3.HeaderMatcher{
			Name: ":method",
			HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_ExactMatch{
				ExactMatch: string(*matcher.Method),
			},
		})
	}

	setEnvoyPathMatcher(matcher, match)
	return match
}

var separatedPathRegex = regexp.MustCompile("^[^?#]+[^?#/]$")

func isValidPathSparated(path string) bool {
	// see envoy docs:
	//	Expect the value to not contain “?“ or “#“ and not to end in “/“
	return separatedPathRegex.MatchString(path)
}

func setEnvoyPathMatcher(match gwv1.HTTPRouteMatch, out *envoy_config_route_v3.RouteMatch) {
	pathType, pathValue := routeutils.ParsePath(match.Path)
	switch pathType {
	case gwv1.PathMatchPathPrefix:
		if !isValidPathSparated(pathValue) {
			out.PathSpecifier = &envoy_config_route_v3.RouteMatch_Prefix{
				Prefix: pathValue,
			}
		} else {
			out.PathSpecifier = &envoy_config_route_v3.RouteMatch_PathSeparatedPrefix{
				PathSeparatedPrefix: pathValue,
			}
		}
	case gwv1.PathMatchExact:
		out.PathSpecifier = &envoy_config_route_v3.RouteMatch_Path{
			Path: pathValue,
		}
	case gwv1.PathMatchRegularExpression:
		out.PathSpecifier = &envoy_config_route_v3.RouteMatch_SafeRegex{
			SafeRegex: regexutils.NewRegexWithProgramSize(pathValue, nil),
		}
	}
}

func envoyHeaderMatcher(in []gwv1.HTTPHeaderMatch) []*envoy_config_route_v3.HeaderMatcher {
	var out []*envoy_config_route_v3.HeaderMatcher
	for _, matcher := range in {

		envoyMatch := &envoy_config_route_v3.HeaderMatcher{
			Name: string(matcher.Name),
		}
		regex := false
		if matcher.Type != nil && *matcher.Type == gwv1.HeaderMatchRegularExpression {
			regex = true
		}

		// TODO: not sure if we should do PresentMatch according to the spec.
		if matcher.Value == "" {
			envoyMatch.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if regex {
				envoyMatch.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_SafeRegexMatch{
					SafeRegexMatch: regexutils.NewRegexWithProgramSize(matcher.Value, nil),
				}
			} else {
				envoyMatch.HeaderMatchSpecifier = &envoy_config_route_v3.HeaderMatcher_ExactMatch{
					ExactMatch: matcher.Value,
				}
			}
		}
		out = append(out, envoyMatch)
	}
	return out
}

func envoyQueryMatcher(in []gwv1.HTTPQueryParamMatch) []*envoy_config_route_v3.QueryParameterMatcher {
	var out []*envoy_config_route_v3.QueryParameterMatcher
	for _, matcher := range in {
		envoyMatch := &envoy_config_route_v3.QueryParameterMatcher{
			Name: string(matcher.Name),
		}
		regex := false
		if matcher.Type != nil && *matcher.Type == gwv1.QueryParamMatchRegularExpression {
			regex = true
		}

		// TODO: not sure if we should do PresentMatch according to the spec.
		if matcher.Value == "" {
			envoyMatch.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if regex {
				envoyMatch.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
					StringMatch: &envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: regexutils.NewRegexWithProgramSize(matcher.Value, nil),
						},
					},
				}
			} else {
				envoyMatch.QueryParameterMatchSpecifier = &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
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

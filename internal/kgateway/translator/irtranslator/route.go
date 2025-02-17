package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/routeutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
)

type httpRouteConfigurationTranslator struct {
	gw       ir.GatewayIR
	listener ir.ListenerIR
	fc       ir.FilterChainCommon

	routeConfigName          string
	reporter                 reports.Reporter
	requireTlsOnVirtualHosts bool
	PluginPass               TranslationPassPlugins
}

func (h *httpRouteConfigurationTranslator) ComputeRouteConfiguration(ctx context.Context, vhosts []*ir.VirtualHost) *envoy_config_route_v3.RouteConfiguration {
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

	return cfg
}

func (h *httpRouteConfigurationTranslator) computeVirtualHosts(ctx context.Context, virtualHosts []*ir.VirtualHost) []*envoy_config_route_v3.VirtualHost {

	var envoyVirtualHosts []*envoy_config_route_v3.VirtualHost
	for _, virtualHost := range virtualHosts {

		envoyVirtualHosts = append(envoyVirtualHosts, h.computeVirtualHost(ctx, virtualHost))
	}
	return envoyVirtualHosts
}

func (h *httpRouteConfigurationTranslator) computeVirtualHost(
	ctx context.Context,
	virtualHost *ir.VirtualHost,
) *envoy_config_route_v3.VirtualHost {
	sanitizedName := utils.SanitizeForEnvoy(ctx, virtualHost.Name, "virtual host")

	var envoyRoutes []*envoy_config_route_v3.Route
	for i, route := range virtualHost.Rules {
		// TODO: not sure if we need listener parent ref here or the http parent ref
		routeReport := h.reporter.Route(route.Parent.SourceObject).ParentRef(&route.ParentRef)
		generatedName := fmt.Sprintf("%s-route-%d", virtualHost.Name, i)
		computedRoute := h.envoyRoutes(ctx, routeReport, route, generatedName)
		if computedRoute != nil {
			envoyRoutes = append(envoyRoutes, computedRoute)
		}
	}
	domains := []string{virtualHost.Hostname}
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
	h.runVhostPlugins(ctx, out)
	for gvk, pols := range h.listener.AttachedPolicies.Policies {
		pass := h.PluginPass[gvk]
		if pass == nil {
			// TODO: should never happen, log error and report condition
			continue
		}
		for _, pol := range pols {
			pctx := &ir.VirtualHostContext{
				Policy: pol.PolicyIr,
			}
			pass.ApplyVhostPlugin(ctx, pctx, out)
			// TODO: check return value, if error returned, log error and report condition
		}
	}

	return out
}

func (h *httpRouteConfigurationTranslator) envoyRoutes(ctx context.Context,
	routeReport reports.ParentRefReporter,
	in ir.HttpRouteRuleMatchIR,
	generatedName string,
) *envoy_config_route_v3.Route {

	out := h.initRoutes(in, generatedName)

	if len(in.Backends) > 0 {
		out.Action = h.translateRouteAction(in, out)
	}
	// run plugins here that may set actoin
	err := h.runRoutePlugins(ctx, routeReport, in, out)

	if err == nil {
		err = validateEnvoyRoute(out)
	}

	if err == nil && out.GetAction() == nil {
		if in.HasChildren {
			return nil
		} else {
			err = errors.New("no action specified")
		}
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
		//  TODO: we currently drop the route which is not good;
		//    we should implement route replacement.
		// out.Reset()
		// out.Action = &envoy_config_route_v3.Route_DirectResponse{
		// 	DirectResponse: &envoy_config_route_v3.DirectResponseAction{
		// 		Status: http.StatusInternalServerError,
		// 	},
		// }
		out = nil

	}

	return out
}

func (h *httpRouteConfigurationTranslator) runVhostPlugins(ctx context.Context, out *envoy_config_route_v3.VirtualHost) {
	attachedPoliciesSlice := []ir.AttachedPolicies{
		h.gw.AttachedHttpPolicies,
		h.listener.AttachedPolicies,
	}
	for _, attachedPolicies := range attachedPoliciesSlice {
		for gk, pols := range attachedPolicies.Policies {
			pass := h.PluginPass[gk]
			if pass == nil {
				// TODO: user error - they attached a non http policy
				continue
			}
			for _, pol := range pols {
				pctx := &ir.VirtualHostContext{
					Policy: pol.PolicyIr,
				}
				pass.ApplyVhostPlugin(ctx, pctx, out)
				// TODO: check return value, if error returned, log error and report condition
			}
		}
	}
}

func (h *httpRouteConfigurationTranslator) runRoutePlugins(ctx context.Context, routeReport reports.ParentRefReporter, in ir.HttpRouteRuleMatchIR, out *envoy_config_route_v3.Route) error {

	// all policies up to listener have been applied as vhost polices; we need to apply the httproute policies and below
	var policiesFromDelegateParent ir.AttachedPolicies
	if in.DelegateParent != nil {
		policiesFromDelegateParent = in.DelegateParent.AttachedPolicies
	}

	attachedPoliciesSlice := []ir.AttachedPolicies{
		in.Parent.AttachedPolicies,
		in.AttachedPolicies,
		in.ExtensionRefs,
		policiesFromDelegateParent,
		// TODO: add policies from the parent's parent recursivly
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
				pctx := &ir.RouteContext{
					Policy: pol.PolicyIr,
					In:     in,
				}
				err := pass.ApplyForRoute(ctx, pctx, out)
				if err != nil {
					errs = append(errs, err)
				}
				// TODO: check return value, if error returned, log error and report condition
			}
		}
	}
	err := errors.Join(errs...)
	if err != nil {
		routeReport.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonIncompatibleFilters,
			Message: err.Error(),
		})
	}

	return err
}

func (h *httpRouteConfigurationTranslator) runBackendPolicies(ctx context.Context, in ir.HttpBackend, pCtx *ir.RouteBackendContext) error {
	var errs []error
	for gk, pols := range in.AttachedPolicies.Policies {
		pass := h.PluginPass[gk]
		if pass == nil {
			// TODO: should never happen, log error and report condition
			continue
		}
		for _, pol := range pols {

			err := pass.ApplyForRouteBackend(ctx, pol.PolicyIr, pCtx)
			if err != nil {
				errs = append(errs, err)
			}
			// TODO: check return value, if error returned, log error and report condition
		}
	}
	return errors.Join(errs...)
}

func (h *httpRouteConfigurationTranslator) translateRouteAction(
	in ir.HttpRouteRuleMatchIR,
	outRoute *envoy_config_route_v3.Route,
) *envoy_config_route_v3.Route_Route {
	var clusters []*envoy_config_route_v3.WeightedCluster_ClusterWeight

	for _, backend := range in.Backends {
		clusterName := backend.Backend.ClusterName

		// get backend for ref - we must do it to make sure we have permissions to access it.
		// also we need the service so we can translate its name correctly.
		cw := &envoy_config_route_v3.WeightedCluster_ClusterWeight{
			Name:   clusterName,
			Weight: wrapperspb.UInt32(backend.Backend.Weight),
		}
		pCtx := ir.RouteBackendContext{
			FilterChainName:  h.fc.FilterChainName,
			Upstream:         backend.Backend.Upstream,
			TypedFiledConfig: &cw.TypedPerFilterConfig,
		}

		h.runBackendPolicies(
			context.TODO(),
			backend,
			&pCtx,
		)
		clusters = append(clusters, cw)
	}

	// TODO: i think envoy nacks if all weights are 0, we should error on that.

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
		if clusters[0].GetTypedPerFilterConfig() != nil {
			if outRoute.GetTypedPerFilterConfig() == nil {
				outRoute.TypedPerFilterConfig = clusters[0].GetTypedPerFilterConfig()
			} else {
				maps.Copy(outRoute.GetTypedPerFilterConfig(), clusters[0].GetTypedPerFilterConfig())
			}
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
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("error %s: %w", r.GetName(), errors.Join(errs...))
}

// creates Envoy routes for each matcher provided on our Gateway route
func (h *httpRouteConfigurationTranslator) initRoutes(
	in ir.HttpRouteRuleMatchIR,
	generatedName string,
) *envoy_config_route_v3.Route {

	//	if len(in.Matches) == 0 {
	//		return []*envoy_config_route_v3.Route{
	//			{
	//				Match: &envoy_config_route_v3.RouteMatch{
	//					PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{Prefix: "/"},
	//				},
	//			},
	//		}
	//	}

	out := &envoy_config_route_v3.Route{
		Match: translateGlooMatcher(in.Match),
	}
	name := in.Name
	if name != "" {
		out.Name = fmt.Sprintf("%s-%s-matcher-%d", generatedName, name, in.MatchIndex)
	} else {
		out.Name = fmt.Sprintf("%s-matcher-%d", generatedName, in.MatchIndex)
	}

	return out
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

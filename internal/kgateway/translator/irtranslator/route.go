package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/routeutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
)

type httpRouteConfigurationTranslator struct {
	gw               ir.GatewayIR
	listener         ir.ListenerIR
	fc               ir.FilterChainCommon
	attachedPolicies ir.AttachedPolicies

	routeConfigName          string
	reporter                 reportssdk.Reporter
	requireTlsOnVirtualHosts bool
	PluginPass               TranslationPassPlugins
	logger                   *slog.Logger
	routeReplacementMode     settings.RouteReplacementMode
}

const WebSocketUpgradeType = "websocket"

func (h *httpRouteConfigurationTranslator) ComputeRouteConfiguration(ctx context.Context, vhosts []*ir.VirtualHost) *envoyroutev3.RouteConfiguration {
	var attachedPolicies ir.AttachedPolicies
	// the policies in order - first listener as they are more specific and thus higher priority.
	// then gateway policies.
	attachedPolicies.Append(h.attachedPolicies, h.gw.AttachedHttpPolicies)
	cfg := &envoyroutev3.RouteConfiguration{
		Name: h.routeConfigName,
	}
	typedPerFilterConfigRoute := ir.TypedFilterConfigMap(map[string]proto.Message{})

	for _, gk := range attachedPolicies.ApplyOrderedGroupKinds() {
		pols := attachedPolicies.Policies[gk]
		pass := h.PluginPass[gk]
		if pass == nil {
			// TODO: user error - they attached a non http policy
			continue
		}
		reportPolicyAcceptanceStatus(h.reporter, h.listener.PolicyAncestorRef, pols...)
		policies, mergeOrigins := mergePolicies(pass, pols)
		for _, pol := range policies {
			pass.ApplyRouteConfigPlugin(ctx, &ir.RouteConfigContext{
				FilterChainName:   h.fc.FilterChainName,
				TypedFilterConfig: typedPerFilterConfigRoute,
				Policy:            pol.PolicyIr,
				GatewayContext:    ir.GatewayContext{GatewayClassName: h.gw.GatewayClassName()},
			}, cfg)
		}
		cfg.Metadata = addMergeOriginsToFilterMetadata(gk, mergeOrigins, cfg.GetMetadata())
		reportPolicyAttachmentStatus(h.reporter, h.listener.PolicyAncestorRef, mergeOrigins, pols...)
	}

	cfg.VirtualHosts = h.computeVirtualHosts(ctx, vhosts)
	cfg.TypedPerFilterConfig = typedPerFilterConfigRoute.ToAnyMap()

	// Gateway API spec requires that port values in HTTP Host headers be ignored when performing a match
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteSpec - hostnames field
	cfg.IgnorePortInHostMatching = true

	//	if mostSpecificVal := h.parentListener.GetRouteOptions().GetMostSpecificHeaderMutationsWins(); mostSpecificVal != nil {
	//		cfg.MostSpecificHeaderMutationsWins = mostSpecificVal.GetValue()
	//	}

	return cfg
}

func (h *httpRouteConfigurationTranslator) computeVirtualHosts(ctx context.Context, virtualHosts []*ir.VirtualHost) []*envoyroutev3.VirtualHost {
	var envoyVirtualHosts []*envoyroutev3.VirtualHost
	for _, virtualHost := range virtualHosts {
		envoyVirtualHosts = append(envoyVirtualHosts, h.computeVirtualHost(ctx, virtualHost))
	}
	return envoyVirtualHosts
}

func (h *httpRouteConfigurationTranslator) computeVirtualHost(
	ctx context.Context,
	virtualHost *ir.VirtualHost,
) *envoyroutev3.VirtualHost {
	sanitizedName := utils.SanitizeForEnvoy(ctx, virtualHost.Name, "virtual host")

	var envoyRoutes []*envoyroutev3.Route
	for i, route := range virtualHost.Rules {
		// TODO: not sure if we need listener parent ref here or the http parent ref
		var routeReport reportssdk.ParentRefReporter = &reports.ParentRefReport{}
		if route.Parent != nil {
			// route may be a fake one that we don't really report,
			// such as in the waypoint translator where we produce
			// synthetic routes if there none are attached to the Gateway/Service.
			routeReport = h.reporter.Route(route.Parent.SourceObject).ParentRef(&route.ParentRef)
		}
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
	var envoyRequireTls envoyroutev3.VirtualHost_TlsRequirementType
	if h.requireTlsOnVirtualHosts {
		// TODO (ilackarms): support external-only TLS
		envoyRequireTls = envoyroutev3.VirtualHost_ALL
	}

	out := &envoyroutev3.VirtualHost{
		Name:       sanitizedName,
		Domains:    domains,
		Routes:     envoyRoutes,
		RequireTls: envoyRequireTls,
	}

	typedPerFilterConfigRoute := ir.TypedFilterConfigMap(map[string]proto.Message{})
	// run the http plugins that are attached to the listener or gateway on the virtual host
	h.runVhostPlugins(ctx, virtualHost, out, typedPerFilterConfigRoute)
	out.TypedPerFilterConfig = typedPerFilterConfigRoute.ToAnyMap()

	return out
}

type backendConfigContext struct {
	typedPerFilterConfigRoute ir.TypedFilterConfigMap
	RequestHeadersToAdd       []*envoycorev3.HeaderValueOption
	RequestHeadersToRemove    []string
	ResponseHeadersToAdd      []*envoycorev3.HeaderValueOption
	ResponseHeadersToRemove   []string
}

func (h *httpRouteConfigurationTranslator) envoyRoutes(
	ctx context.Context,
	routeReport reportssdk.ParentRefReporter,
	in ir.HttpRouteRuleMatchIR,
	generatedName string,
) *envoyroutev3.Route {
	out := h.initRoutes(in, generatedName)

	backendConfigCtx := backendConfigContext{typedPerFilterConfigRoute: ir.TypedFilterConfigMap(map[string]proto.Message{})}
	if len(in.Backends) == 1 {
		// if there's only one backend, we need to reuse typedPerFilterConfigRoute in both translateRouteAction and runRoutePlugins
		out.Action = h.translateRouteAction(ctx, in, out, &backendConfigCtx)
	} else if len(in.Backends) > 0 {
		// If there is more than one backend, we translate the backends as WeightedClusters and each weighted cluster
		// will have a TypedPerFilterConfig that overrides the parent route-level config.
		out.Action = h.translateRouteAction(ctx, in, out, nil)
	}

	// run plugins here that may set action
	routeProcessingErr := h.runRoutePlugins(ctx, in, out, backendConfigCtx.typedPerFilterConfigRoute)
	if routeProcessingErr == nil {
		routeProcessingErr = validateEnvoyRoute(out)
	}

	// apply typed per filter config from translating route action and route plugins
	typedPerFilterConfig := backendConfigCtx.typedPerFilterConfigRoute.ToAnyMap()
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = typedPerFilterConfig
	} else {
		for k, v := range typedPerFilterConfig {
			if _, exists := out.GetTypedPerFilterConfig()[k]; !exists {
				out.GetTypedPerFilterConfig()[k] = v
			}
		}
	}

	// If routeProcessingErr is nil, check if the route has an action for non-delegating routes
	// to treat this as an error that should result in route replacement.
	// A delegating(parent) route does not need to have an output Action on itself,
	// so do not treat it as an error
	if routeProcessingErr == nil && out.GetAction() == nil && !in.Delegates {
		routeProcessingErr = errors.New("no action specified")
	}

	// routeAcceptanceErr is used to set the Accepted=false,Reason=RouteRuleDropped condition on the route
	routeAcceptanceErr := errors.Join(routeProcessingErr, in.RouteAcceptanceError)

	// routeReplacementErr is used to replace the route with a direct response
	routeReplacementErr := errors.Join(routeProcessingErr, in.RouteReplacementError, routeAcceptanceErr)

	// If this is a delegating(parent) route rule and it has no other errors
	// return a nil route since delegating parent route rules are expected to have no action set
	if in.Delegates && routeAcceptanceErr == nil && routeReplacementErr == nil {
		return nil
	}

	// If routeReplacementErr is set, we need to replace the route with a direct response
	if routeReplacementErr != nil {
		h.logger.Debug("invalid route", "error", routeReplacementErr)

		// If routeAcceptanceErr is set, report Accepted=False with Reason=RouteRuleDropped
		if routeAcceptanceErr != nil {
			routeReport.SetCondition(reportssdk.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteConditionReason(reportssdk.RouteRuleDroppedReason),
				Message: fmt.Sprintf("Dropped Rule (%d): %v", in.MatchIndex, routeAcceptanceErr),
			})
		}

		switch h.routeReplacementMode {
		case settings.RouteReplacementStandard, settings.RouteReplacementStrict:
			// Unset the TypedPerFilterConfig when the route is replaced with a direct response
			out.TypedPerFilterConfig = nil
			// Replace invalid route with a direct response
			out.Action = &envoyroutev3.Route_DirectResponse{
				DirectResponse: &envoyroutev3.DirectResponseAction{
					Status: http.StatusInternalServerError,
					Body: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineString{
							InlineString: `invalid route configuration detected and replaced with a direct response.`,
						},
					},
				},
			}
			return out
		default:
			// Drop the route entirely (legacy behavior, will be removed in the future)
			return nil
		}
	}

	out.RequestHeadersToAdd = append(out.GetRequestHeadersToAdd(), backendConfigCtx.RequestHeadersToAdd...)
	out.RequestHeadersToRemove = append(out.GetRequestHeadersToRemove(), backendConfigCtx.RequestHeadersToRemove...)
	out.ResponseHeadersToAdd = append(out.GetResponseHeadersToAdd(), backendConfigCtx.ResponseHeadersToAdd...)
	out.ResponseHeadersToRemove = append(out.GetResponseHeadersToRemove(), backendConfigCtx.ResponseHeadersToRemove...)

	return out
}

func (h *httpRouteConfigurationTranslator) runVhostPlugins(
	ctx context.Context,
	virtualHost *ir.VirtualHost,
	out *envoyroutev3.VirtualHost,
	typedPerFilterConfig ir.TypedFilterConfigMap,
) {
	for _, gk := range virtualHost.AttachedPolicies.ApplyOrderedGroupKinds() {
		pols := virtualHost.AttachedPolicies.Policies[gk]
		pass := h.PluginPass[gk]
		if pass == nil {
			// TODO: user error - they attached a non http policy
			continue
		}
		reportPolicyAcceptanceStatus(h.reporter, h.listener.PolicyAncestorRef, pols...)
		policies, mergeOrigins := mergePolicies(pass, pols)
		for _, pol := range policies {
			pctx := &ir.VirtualHostContext{
				Policy:            pol.PolicyIr,
				TypedFilterConfig: typedPerFilterConfig,
				FilterChainName:   h.fc.FilterChainName,
				GatewayContext:    ir.GatewayContext{GatewayClassName: h.gw.GatewayClassName()},
			}
			pass.ApplyVhostPlugin(ctx, pctx, out)
		}
		out.Metadata = addMergeOriginsToFilterMetadata(gk, mergeOrigins, out.GetMetadata())
		reportPolicyAttachmentStatus(h.reporter, h.listener.PolicyAncestorRef, mergeOrigins, pols...)
	}
}

func (h *httpRouteConfigurationTranslator) runRoutePlugins(
	ctx context.Context,
	in ir.HttpRouteRuleMatchIR,
	out *envoyroutev3.Route,
	typedPerFilterConfig ir.TypedFilterConfigMap,
) error {
	// all policies up to listener have been applied as vhost polices; we need to apply the httproute policies and below
	//
	// NOTE: AttachedPolicies must have policies in the ordered by hierarchy from leaf to root in the delegation chain where
	// each level has policies ordered by rule level policies before entire route level policies.
	// A policy appearing earlier in the list has a higher priority than a policy appearing later in the list during merging.

	var attachedPolicies ir.AttachedPolicies

	// rule-level policies in priority order (high to low)
	attachedPolicies.Append(in.ExtensionRefs, in.AttachedPolicies)

	// route-level policy
	if in.Parent != nil {
		attachedPolicies.Append(in.Parent.AttachedPolicies)
	}

	hierarchicalPriority := 0
	delegatingParent := in.DelegatingParent
	for delegatingParent != nil {
		// parent policies are lower in priority by default, so mark them with their relative priority
		hierarchicalPriority--
		attachedPolicies.AppendWithPriority(hierarchicalPriority,
			delegatingParent.ExtensionRefs, delegatingParent.AttachedPolicies, delegatingParent.Parent.AttachedPolicies)
		delegatingParent = delegatingParent.DelegatingParent
	}

	var errs []error
	for _, gk := range attachedPolicies.ApplyOrderedGroupKinds() {
		pols := attachedPolicies.Policies[gk]
		pass := h.PluginPass[gk]
		if pass == nil {
			// TODO: should never happen, log error and report condition
			continue
		}
		pctx := &ir.RouteContext{
			GatewayContext:    ir.GatewayContext{GatewayClassName: h.gw.GatewayClassName()},
			FilterChainName:   h.fc.FilterChainName,
			In:                in,
			TypedFilterConfig: typedPerFilterConfig,
		}
		reportPolicyAcceptanceStatus(h.reporter, h.listener.PolicyAncestorRef, pols...)
		policies, mergeOrigins := mergePolicies(pass, pols)
		for _, pol := range policies {
			// Builtin policies use InheritedPolicyPriority
			pctx.InheritedPolicyPriority = pol.InheritedPolicyPriority

			// skip plugin application if we encountered any errors while constructing
			// the policy IR.
			if len(pol.Errors) > 0 {
				errs = append(errs, pol.Errors...)
				continue
			}

			pctx.Policy = pol.PolicyIr
			err := pass.ApplyForRoute(ctx, pctx, out)
			if err != nil {
				errs = append(errs, err)
			}
		}
		out.Metadata = addMergeOriginsToFilterMetadata(gk, mergeOrigins, out.GetMetadata())
		reportPolicyAttachmentStatus(h.reporter, h.listener.PolicyAncestorRef, mergeOrigins, pols...)
	}

	return errors.Join(errs...)
}

func mergePolicies(pass *TranslationPass, policies []ir.PolicyAtt) ([]ir.PolicyAtt, ir.MergeOrigins) {
	if pass.MergePolicies != nil {
		mergedPolicy := pass.MergePolicies(policies)
		merged := [1]ir.PolicyAtt{mergedPolicy}
		return merged[:], mergedPolicy.MergeOrigins
	}

	return policies, nil
}

func (h *httpRouteConfigurationTranslator) runBackendPolicies(ctx context.Context, in ir.HttpBackend, pCtx *ir.RouteBackendContext) error {
	var errs []error
	for _, gk := range in.AttachedPolicies.ApplyOrderedGroupKinds() {
		pols := in.AttachedPolicies.Policies[gk]
		pass := h.PluginPass[gk]
		if pass == nil {
			// TODO: should never happen, log error and report condition
			continue
		}
		reportPolicyAcceptanceStatus(h.reporter, h.listener.PolicyAncestorRef, pols...)
		policies, _ := mergePolicies(pass, pols)
		for _, pol := range policies {
			// Policy on extension ref
			err := pass.ApplyForRouteBackend(ctx, pol.PolicyIr, pCtx)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (h *httpRouteConfigurationTranslator) runBackend(ctx context.Context, in ir.HttpBackend, pCtx *ir.RouteBackendContext, outRoute *envoyroutev3.Route) error {
	var errs []error
	if in.Backend.BackendObject != nil {
		backendPass := h.PluginPass[in.Backend.BackendObject.GetGroupKind()]
		if backendPass != nil {
			err := backendPass.ApplyForBackend(ctx, pCtx, in, outRoute)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	// TODO: check return value, if error returned, log error and report condition
	return errors.Join(errs...)
}

func (h *httpRouteConfigurationTranslator) translateRouteAction(
	ctx context.Context,
	in ir.HttpRouteRuleMatchIR,
	outRoute *envoyroutev3.Route,
	parentBackendConfigCtx *backendConfigContext,
) *envoyroutev3.Route_Route {
	var clusters []*envoyroutev3.WeightedCluster_ClusterWeight

	for _, backend := range in.Backends {
		clusterName := backend.Backend.ClusterName

		// get backend for ref - we must do it to make sure we have permissions to access it.
		// also we need the service so we can translate its name correctly.
		cw := &envoyroutev3.WeightedCluster_ClusterWeight{
			Name:   clusterName,
			Weight: wrapperspb.UInt32(backend.Backend.Weight),
		}

		backendConfigCtx := parentBackendConfigCtx
		if parentBackendConfigCtx == nil {
			backendConfigCtx = &backendConfigContext{typedPerFilterConfigRoute: ir.TypedFilterConfigMap(map[string]proto.Message{})}
		}

		pCtx := ir.RouteBackendContext{
			GatewayContext:    ir.GatewayContext{GatewayClassName: h.gw.GatewayClassName()},
			FilterChainName:   h.fc.FilterChainName,
			Backend:           backend.Backend.BackendObject,
			TypedFilterConfig: backendConfigCtx.typedPerFilterConfigRoute,
		}

		// non attached policy translation
		err := h.runBackend(
			ctx,
			backend,
			&pCtx,
			outRoute,
		)
		if err != nil {
			// TODO: error on status
			h.logger.Error("error processing backends", "error", err)
		}
		err = h.runBackendPolicies(
			ctx,
			backend,
			&pCtx,
		)
		if err != nil {
			// TODO: error on status
			h.logger.Error("error processing backends with policies", "error", err)
		}

		backendConfigCtx.RequestHeadersToAdd = pCtx.RequestHeadersToAdd
		backendConfigCtx.RequestHeadersToRemove = pCtx.RequestHeadersToRemove
		backendConfigCtx.ResponseHeadersToAdd = pCtx.ResponseHeadersToAdd
		backendConfigCtx.ResponseHeadersToRemove = pCtx.ResponseHeadersToRemove

		// Translating weighted clusters needs the typed per filter config on each cluster
		cw.TypedPerFilterConfig = backendConfigCtx.typedPerFilterConfigRoute.ToAnyMap()
		cw.RequestHeadersToAdd = backendConfigCtx.RequestHeadersToAdd
		cw.RequestHeadersToRemove = backendConfigCtx.RequestHeadersToRemove
		cw.ResponseHeadersToAdd = backendConfigCtx.ResponseHeadersToAdd
		cw.ResponseHeadersToRemove = backendConfigCtx.ResponseHeadersToRemove
		clusters = append(clusters, cw)
	}

	action := outRoute.GetRoute()
	if action == nil {
		action = &envoyroutev3.RouteAction{
			ClusterNotFoundResponseCode: envoyroutev3.RouteAction_INTERNAL_SERVER_ERROR,
		}
	}

	routeAction := &envoyroutev3.Route_Route{
		Route: action,
	}
	switch len(clusters) {
	// case 0:
	// TODO: we should never get here
	case 1:
		// Only set the cluster name if unspecified since a plugin may have set it.
		if action.GetCluster() == "" {
			action.ClusterSpecifier = &envoyroutev3.RouteAction_Cluster{
				Cluster: clusters[0].GetName(),
			}
		}
		// Skip setting the typed per filter config here, set it in the envoyRoutes() after runRoutePlugins runs

	default:
		// Only set weighted clusters if unspecified since a plugin may have set it.
		if action.GetWeightedClusters() == nil {
			action.ClusterSpecifier = &envoyroutev3.RouteAction_WeightedClusters{
				WeightedClusters: &envoyroutev3.WeightedCluster{
					Clusters: clusters,
				},
			}
		}
	}

	for _, backend := range in.Backends {
		if back := backend.Backend.BackendObject; back != nil && back.AppProtocol == ir.WebSocketAppProtocol {
			// add websocket upgrade if not already present
			if !slices.ContainsFunc(action.GetUpgradeConfigs(), func(uc *envoyroutev3.RouteAction_UpgradeConfig) bool {
				return uc.GetUpgradeType() == WebSocketUpgradeType
			}) {
				action.UpgradeConfigs = append(action.GetUpgradeConfigs(), &envoyroutev3.RouteAction_UpgradeConfig{
					UpgradeType: WebSocketUpgradeType,
				})
			}
		}
	}
	return routeAction
}

func validateEnvoyRoute(r *envoyroutev3.Route) error {
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
	validateWeightedClusters(route.GetWeightedClusters().GetClusters(), &errs)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("error %s: %w", r.GetName(), errors.Join(errs...))
}

func validateWeightedClusters(clusters []*envoyroutev3.WeightedCluster_ClusterWeight, errs *[]error) {
	if len(clusters) == 0 {
		return
	}

	allZeroWeight := true
	for _, cluster := range clusters {
		if cluster.GetWeight().GetValue() > 0 {
			allZeroWeight = false
			break
		}
	}
	if allZeroWeight {
		*errs = append(*errs, errors.New("All backend weights are 0. At least one backendRef in the HTTPRoute rule must specify a non-zero weight"))
	}
}

// creates Envoy routes for each matcher provided on our Gateway route
func (h *httpRouteConfigurationTranslator) initRoutes(
	in ir.HttpRouteRuleMatchIR,
	generatedName string,
) *envoyroutev3.Route {
	//	if len(in.Matches) == 0 {
	//		return []*envoyroutev3.Route{
	//			{
	//				Match: &envoyroutev3.RouteMatch{
	//					PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: "/"},
	//				},
	//			},
	//		}
	//	}

	out := &envoyroutev3.Route{
		Match: translateMatcher(in.Match),
	}
	name := in.Name
	if name != "" {
		out.Name = fmt.Sprintf("%s-%s-matcher-%d", generatedName, name, in.MatchIndex)
	} else {
		out.Name = fmt.Sprintf("%s-matcher-%d", generatedName, in.MatchIndex)
	}

	return out
}

func translateMatcher(matcher gwv1.HTTPRouteMatch) *envoyroutev3.RouteMatch {
	match := &envoyroutev3.RouteMatch{
		Headers:         envoyHeaderMatcher(matcher.Headers),
		QueryParameters: envoyQueryMatcher(matcher.QueryParams),
	}
	if matcher.Method != nil {
		match.Headers = append(match.GetHeaders(), &envoyroutev3.HeaderMatcher{
			Name: ":method",
			HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_StringMatch{
				StringMatch: &envoy_type_matcher_v3.StringMatcher{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
						Exact: string(*matcher.Method),
					},
				},
			},
		})
	}

	setEnvoyPathMatcher(matcher, match)
	return match
}

var separatedPathRegex = regexp.MustCompile("^[^?#]+[^?#/]$")

func isValidPathSparated(path string) bool {
	// see envoy docs:
	//	Expect the value to not contain "?" or "#" and not to end in "/"
	return separatedPathRegex.MatchString(path)
}

func setEnvoyPathMatcher(match gwv1.HTTPRouteMatch, out *envoyroutev3.RouteMatch) {
	pathType, pathValue := routeutils.ParsePath(match.Path)
	switch pathType {
	case gwv1.PathMatchPathPrefix:
		if !isValidPathSparated(pathValue) {
			out.PathSpecifier = &envoyroutev3.RouteMatch_Prefix{
				Prefix: pathValue,
			}
		} else {
			out.PathSpecifier = &envoyroutev3.RouteMatch_PathSeparatedPrefix{
				PathSeparatedPrefix: pathValue,
			}
		}
	case gwv1.PathMatchExact:
		out.PathSpecifier = &envoyroutev3.RouteMatch_Path{
			Path: pathValue,
		}
	case gwv1.PathMatchRegularExpression:
		out.PathSpecifier = &envoyroutev3.RouteMatch_SafeRegex{
			SafeRegex: regexutils.NewRegexWithProgramSize(pathValue, nil),
		}
	}
}

func envoyHeaderMatcher(in []gwv1.HTTPHeaderMatch) []*envoyroutev3.HeaderMatcher {
	var out []*envoyroutev3.HeaderMatcher
	for _, matcher := range in {
		envoyMatch := &envoyroutev3.HeaderMatcher{
			Name: string(matcher.Name),
		}
		regex := false
		if matcher.Type != nil && *matcher.Type == gwv1.HeaderMatchRegularExpression {
			regex = true
		}

		// TODO: not sure if we should do PresentMatch according to the spec.
		if matcher.Value == "" {
			envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if regex {
				envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_StringMatch{
					StringMatch: &envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: regexutils.NewRegexWithProgramSize(matcher.Value, nil),
						},
					},
				}
			} else {
				envoyMatch.HeaderMatchSpecifier = &envoyroutev3.HeaderMatcher_StringMatch{
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

func envoyQueryMatcher(in []gwv1.HTTPQueryParamMatch) []*envoyroutev3.QueryParameterMatcher {
	var out []*envoyroutev3.QueryParameterMatcher
	for _, matcher := range in {
		envoyMatch := &envoyroutev3.QueryParameterMatcher{
			Name: string(matcher.Name),
		}
		regex := false
		if matcher.Type != nil && *matcher.Type == gwv1.QueryParamMatchRegularExpression {
			regex = true
		}

		// TODO: not sure if we should do PresentMatch according to the spec.
		if matcher.Value == "" {
			envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_PresentMatch{
				PresentMatch: true,
			}
		} else {
			if regex {
				envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_StringMatch{
					StringMatch: &envoy_type_matcher_v3.StringMatcher{
						MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: regexutils.NewRegexWithProgramSize(matcher.Value, nil),
						},
					},
				}
			} else {
				envoyMatch.QueryParameterMatchSpecifier = &envoyroutev3.QueryParameterMatcher_StringMatch{
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

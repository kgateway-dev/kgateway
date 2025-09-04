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
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

type httpRouteConfigurationTranslator struct {
	gw               ir.GatewayIR
	listener         ir.ListenerIR
	fc               ir.FilterChainCommon
	attachedPolicies ir.AttachedPolicies

	routeConfigName          string
	reporter                 reportssdk.Reporter
	requireTlsOnVirtualHosts bool
	pluginPass               TranslationPassPlugins
	logger                   *slog.Logger
	routeReplacementMode     settings.RouteReplacementMode
	validator                validator.Validator
}

const (
	// webSocketUpgradeType is the type of upgrade to use for WebSocket connections.
	webSocketUpgradeType = "websocket"
	// directResponseActionBody is the body of the direct response action for replaced
	// routes.
	directResponseActionBody = `invalid route configuration detected and replaced with a direct response.`
)

func (h *httpRouteConfigurationTranslator) ComputeRouteConfiguration(
	ctx context.Context,
	vhosts []*ir.VirtualHost,
) *envoyroutev3.RouteConfiguration {
	cfg := &envoyroutev3.RouteConfiguration{
		Name: h.routeConfigName,
	}

	// Compute virtual hosts from the IR. In listener merging scenarios, vhosts contains
	// all virtual hosts from multiple listeners that share the same port. Each distinct
	// hostname on each HTTPRoute attached to a listener will be a separate vhost.
	cfg.VirtualHosts = h.computeVirtualHosts(ctx, vhosts)

	// Gateway API spec requires that port values in HTTP Host headers be ignored when performing a match
	// See https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteSpec - hostnames field
	cfg.IgnorePortInHostMatching = true

	// Combine policies by precedence (listener, then gateway) so policies with the same
	// GK end up in a single slice. This is necessary to make sure that merging attached
	// policies with the same GK across different levels of the config hierarchy works correctly.
	var attachedPolicies ir.AttachedPolicies
	attachedPolicies.Append(h.attachedPolicies, h.gw.AttachedHttpPolicies)

	// Apply plugins and report status for each GK attached to the route config.
	var errs []error
	typedPerFilterConfigRoute := ir.TypedFilterConfigMap(map[string]proto.Message{})
	for _, gk := range attachedPolicies.ApplyOrderedGroupKinds() {
		pols := attachedPolicies.Policies[gk]
		pass := h.pluginPass[gk]
		if pass == nil {
			continue
		}
		policies, mergeOrigins := mergePolicies(pass, pols)
		reportPolicyAcceptanceStatus(h.reporter, h.listener.PolicyAncestorRef, pols...)
		reportRouteConfigPolicyErrors(h.reporter, h.gw, h.routeConfigName, pols...)
		for _, pol := range policies {
			if len(pol.Errors) > 0 {
				errs = append(errs, pol.Errors...)
				continue
			}
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
	if len(errs) > 0 {
		// Anytime we encounter any errors while computing the RC or there's invalid policy
		// attached to the RC (via Gateway or HTTPS listener), we need to replace the entire
		// RC with a synthetic vhost that returns a 500 error for all traffic.
		h.logger.Error("error applying route config plugins", "error", errors.Join(errs...))
		cfg.VirtualHosts = []*envoyroutev3.VirtualHost{setFallBackConfig("default", "*")}
		return cfg
	}
	cfg.TypedPerFilterConfig = typedPerFilterConfigRoute.ToAnyMap()

	return cfg
}

func (h *httpRouteConfigurationTranslator) computeVirtualHosts(
	ctx context.Context,
	virtualHosts []*ir.VirtualHost,
) []*envoyroutev3.VirtualHost {
	envoyVirtualHosts := make([]*envoyroutev3.VirtualHost, 0, len(virtualHosts))
	for _, virtualHost := range virtualHosts {
		envoyVirtualHosts = append(envoyVirtualHosts, h.computeVirtualHost(ctx, virtualHost))
	}
	return envoyVirtualHosts
}

// computeVirtualHost translates one IR virtual host into an Envoy virtual host and
// applies HTTP-listener attached policies at vhost scope. In the case of shared HTTP
// ports, this is the isolation boundary: failures here replace only this vhost with
// a 500 direct response while preserving name and domains.
func (h *httpRouteConfigurationTranslator) computeVirtualHost(
	ctx context.Context,
	virtualHost *ir.VirtualHost,
) *envoyroutev3.VirtualHost {
	sanitizedName := utils.SanitizeForEnvoy(ctx, virtualHost.Name, "virtual host")

	var envoyRoutes []*envoyroutev3.Route
	for i, route := range virtualHost.Rules {
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
	// run any plugins attached to an HTTP-based listener on the computed vhost.
	if err := h.runVhostPlugins(ctx, virtualHost, out, typedPerFilterConfigRoute); err != nil {
		h.logger.Error("error running vhost plugins", "error", err)
		reporter := virtualHost.ParentRef.GetParentReporter(h.reporter)
		reporter.Listener(&virtualHost.ParentRef.Listener).SetCondition(reportssdk.ListenerCondition{
			Type:    gwv1.ListenerConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  reportssdk.ListenerReplacedReason,
			Message: err.Error(),
		})
		// replace the computed vhost with a fallback that preserves the original vhost identity
		return setFallBackConfig(sanitizedName, domains[0])
	}
	out.TypedPerFilterConfig = typedPerFilterConfigRoute.ToAnyMap()

	return out
}

// setFallBackConfig creates a synthetic, catch-all virtual host that returns 500 errors
// for all traffic that references this vhost.
func setFallBackConfig(name, domain string) *envoyroutev3.VirtualHost {
	return &envoyroutev3.VirtualHost{
		Domains: []string{domain},
		Name:    name,
		Routes: []*envoyroutev3.Route{{
			Match: &envoyroutev3.RouteMatch{
				PathSpecifier: &envoyroutev3.RouteMatch_Prefix{
					Prefix: "/",
				},
			},
			Action: &envoyroutev3.Route_DirectResponse{
				DirectResponse: &envoyroutev3.DirectResponseAction{
					Status: http.StatusInternalServerError,
					Body: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineString{
							InlineString: directResponseActionBody,
						},
					},
				},
			},
		}},
	}
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
		// If there's only one backend, we need to reuse typedPerFilterConfigRoute in both translateRouteAction and runRoutePlugins
		out.Action = h.translateRouteAction(ctx, in, out, &backendConfigCtx)
	} else if len(in.Backends) > 0 {
		// If there is more than one backend, we translate the backends as WeightedClusters and each weighted cluster
		// will have a TypedPerFilterConfig that overrides the parent route-level config.
		out.Action = h.translateRouteAction(ctx, in, out, nil)
	}

	// Run plugins here that may set action. Handle the routeProcessingErr error later.
	routeProcessingErr := h.runRoutePlugins(ctx, in, out, backendConfigCtx.typedPerFilterConfigRoute)

	// Apply typed per filter config from translating route action and route plugins
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

	// Apply the headers to the route
	out.RequestHeadersToAdd = append(out.GetRequestHeadersToAdd(), backendConfigCtx.RequestHeadersToAdd...)
	out.RequestHeadersToRemove = append(out.GetRequestHeadersToRemove(), backendConfigCtx.RequestHeadersToRemove...)
	out.ResponseHeadersToAdd = append(out.GetResponseHeadersToAdd(), backendConfigCtx.ResponseHeadersToAdd...)
	out.ResponseHeadersToRemove = append(out.GetResponseHeadersToRemove(), backendConfigCtx.ResponseHeadersToRemove...)

	// If routeProcessingErr is nil, check if the route has an action for non-delegating routes
	// to treat this as an error that should result in route replacement.
	// A delegating(parent) route does not need to have an output Action on itself,
	// so do not treat it as an error
	if routeProcessingErr == nil && out.GetAction() == nil && !in.Delegates {
		routeProcessingErr = errors.New("no action specified")
	}

	// If there are no errors, validate the route will not be rejected by the xDS server.
	if routeProcessingErr == nil {
		routeProcessingErr = validateRoute(ctx, out, h.validator, h.routeReplacementMode)
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

	// For invalid matchers, we drop the route entirely instead of replacing it with a synthetic matcher.
	if routeAcceptanceErr != nil && errors.Is(routeAcceptanceErr, ErrInvalidMatcher) {
		h.logger.Info("invalid matcher", "error", routeAcceptanceErr)
		routeReport.SetCondition(reportssdk.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteConditionReason(reportssdk.RouteRuleDroppedReason),
			Message: fmt.Sprintf("Dropped Rule (%d): %v", in.MatchIndex, routeAcceptanceErr),
		})
		return nil
	}

	// If routeReplacementErr is set, we need to replace the route with a direct response
	if routeReplacementErr != nil {
		h.logger.Debug("invalid route", "error", routeReplacementErr)

		// If routeAcceptanceErr is set, report Accepted=False with Reason=RouteRuleReplaced
		if routeAcceptanceErr != nil {
			routeReport.SetCondition(reportssdk.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteConditionReason(reportssdk.RouteRuleReplacedReason),
				Message: fmt.Sprintf("Replaced Rule (%d): %v", in.MatchIndex, routeAcceptanceErr),
			})
		}

		if h.routeReplacementMode == settings.RouteReplacementStandard || h.routeReplacementMode == settings.RouteReplacementStrict {
			// Clear all headers and filter configs when the route is replaced with a direct response
			out.TypedPerFilterConfig = nil
			out.RequestHeadersToAdd = nil
			out.RequestHeadersToRemove = nil
			out.ResponseHeadersToAdd = nil
			out.ResponseHeadersToRemove = nil
			// Replace invalid route with a direct response
			out.Action = &envoyroutev3.Route_DirectResponse{
				DirectResponse: &envoyroutev3.DirectResponseAction{
					Status: http.StatusInternalServerError,
					Body: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineString{
							InlineString: directResponseActionBody,
						},
					},
				},
			}
			return out
		}
	}

	return out
}

func (h *httpRouteConfigurationTranslator) runVhostPlugins(
	ctx context.Context,
	virtualHost *ir.VirtualHost,
	out *envoyroutev3.VirtualHost,
	typedPerFilterConfig ir.TypedFilterConfigMap,
) error {
	// Apply HTTP-listener-attached policies at vhost scope. On shared HTTP ports,
	// this is the isolation boundary: failures here replace only this vhost with
	// a 500 direct response (preserving Name and Domains). Policies that require
	// HCM/global knobs must be handled at RouteConfiguration scope instead.
	var errs []error
	for _, gk := range virtualHost.AttachedPolicies.ApplyOrderedGroupKinds() {
		pols := virtualHost.AttachedPolicies.Policies[gk]
		pass := h.pluginPass[gk]
		if pass == nil {
			continue
		}
		reportPolicyAcceptanceStatus(h.reporter, h.listener.PolicyAncestorRef, pols...)
		policies, mergeOrigins := mergePolicies(pass, pols)
		for _, pol := range policies {
			if len(pol.Errors) > 0 {
				errs = append(errs, pol.Errors...)
				continue
			}
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
	return errors.Join(errs...)
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
		pass := h.pluginPass[gk]
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
		pass := h.pluginPass[gk]
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
		backendPass := h.pluginPass[in.Backend.BackendObject.GetGroupKind()]
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
				return uc.GetUpgradeType() == webSocketUpgradeType
			}) {
				action.UpgradeConfigs = append(action.GetUpgradeConfigs(), &envoyroutev3.RouteAction_UpgradeConfig{
					UpgradeType: webSocketUpgradeType,
				})
			}
		}
	}
	return routeAction
}

// creates Envoy routes for each matcher provided on our Gateway route
func (h *httpRouteConfigurationTranslator) initRoutes(
	in ir.HttpRouteRuleMatchIR,
	generatedName string,
) *envoyroutev3.Route {
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

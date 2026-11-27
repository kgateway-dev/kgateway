package trafficpolicy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	kgateway "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	ExtAuthGlobalDisableFilterName              = "global_disable/ext_auth"
	ExtAuthGlobalDisableFilterMetadataNamespace = "dev.kgateway.disable_ext_auth"
	globalFilterDisableMetadataKey              = "disable"
	extauthFilterNamePrefix                     = "ext_auth"

	AuthPolicyMetadataNamespace = "dev.kgateway.auth_policy"
	AuthSucceededMetadataKey    = "auth_succeeded"
	ExtAuthEnabledFilterName    = "ext_auth_enabled"

	// extAuthPostAuthNSuffix names the ext-auth instance that runs after JWT. Routes
	// that do not opt into metadata keep using the canonical name before JWT.
	extAuthPostAuthNSuffix = "/post-authn"

	extAuthClearRouteCacheMetadataConflict = "extauth: clearRouteCache cannot be combined with metadataContextNamespaces"
)

var ExtAuthzEnabledMetadataMatcher = &envoy_matcher_v3.MetadataMatcher{
	Filter: ExtAuthGlobalDisableFilterMetadataNamespace,
	Invert: true,
	Path: []*envoy_matcher_v3.MetadataMatcher_PathSegment{
		{
			Segment: &envoy_matcher_v3.MetadataMatcher_PathSegment_Key{
				Key: globalFilterDisableMetadataKey,
			},
		},
	},
	Value: &envoy_matcher_v3.ValueMatcher{
		MatchPattern: &envoy_matcher_v3.ValueMatcher_BoolMatch{
			BoolMatch: true,
		},
	},
}

type extAuthIR struct {
	// perProviderConfig is a list of ExtAuth providers that may be the result of a
	// merge between policy IRs attached to the same resource or just a single IR
	// when representing a singular policy before a merge
	perProviderConfig   []*perProviderExtAuthConfig
	disableAllProviders bool
	// providerNames is used to track duplicates during policy merging,
	// and has no relevance to the policy config, so it can be excluded from Equals
	// +noKrtEquals
	providerNames sets.Set[string]
	// sourcePolicies maps the IDs of the policies that contributed extAuth to
	// this IR to their identity. Merging drops PolicyAtt.PolicyRef, so this
	// preserves the ability to report status on the original policies.
	// It is merge bookkeeping, not policy config.
	// +noKrtEquals
	sourcePolicies map[string]extAuthPolicyIdentity
}

type perProviderExtAuthConfig struct {
	provider       *TrafficPolicyGatewayExtensionIR
	perRouteConfig *envoy_ext_authz_v3.ExtAuthzPerRoute
	// mergeOrigins holds the IDs of the policies this config came from. It is only set
	// on copies made while merging, so that a field's merge origins can be rebuilt from
	// the configs that survive a merge; nil means the config has not been merged yet.
	// It is merge bookkeeping, not policy config.
	// +noKrtEquals
	mergeOrigins sets.Set[string]
}

var _ PolicySubIR = &extAuthIR{}

// Equals compares two ExtAuthIR instances for equality
func (e *extAuthIR) Equals(other PolicySubIR) bool {
	otherExtAuth, ok := other.(*extAuthIR)
	if !ok {
		return false
	}
	if e == nil || otherExtAuth == nil {
		return e == nil && otherExtAuth == nil
	}
	if e.disableAllProviders != otherExtAuth.disableAllProviders {
		return false
	}
	if !slices.EqualFunc(e.perProviderConfig, otherExtAuth.perProviderConfig, func(a, b *perProviderExtAuthConfig) bool {
		// compare perRouteConfig
		return proto.Equal(a.perRouteConfig, b.perRouteConfig) &&
			// compare provider config
			cmputils.CompareWithNils(a.provider, b.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
				return a.Equals(*b)
			})
	}) {
		return false
	}

	return true
}

func (e *extAuthIR) Validate() error {
	if e == nil {
		return nil
	}
	usesPostAuthNMetadata := false
	hasRouteCacheClearer := false
	for _, p := range e.perProviderConfig {
		if p.perRouteConfig != nil {
			if err := p.perRouteConfig.ValidateAll(); err != nil {
				return err
			}
		}
		if p.provider != nil {
			if err := p.provider.Validate(); err != nil {
				return err
			}
			extAuth := p.provider.ExtAuth
			if extAuth != nil {
				usesPostAuthNMetadata = usesPostAuthNMetadata || extAuthProviderRequestsMetadata(extAuth)
				hasRouteCacheClearer = hasRouteCacheClearer || extAuth.GetClearRouteCache()
			}
		}
	}
	// A route cache clear after JWT can select a destination whose JWT policy has
	// not run. This also rejects the cross-provider form of the CRD validation:
	// one provider requests post-AuthN metadata while another clears the route
	// cache after the policies have been merged.
	if usesPostAuthNMetadata && hasRouteCacheClearer {
		return errors.New(extAuthClearRouteCacheMetadataConflict)
	}
	return nil
}

// constructExtAuth constructs the external authentication policy IR from the policy specification.
func constructExtAuth(
	krtctx krt.HandlerContext,
	in *kgateway.TrafficPolicy,
	fetchGatewayExtension FetchGatewayExtensionFunc,
	out *trafficPolicySpecIr,
) error {
	spec := in.Spec.ExtAuth
	if spec == nil {
		return nil
	}

	if spec.Disable != nil {
		out.extAuth = &extAuthIR{
			disableAllProviders: true,
		}
		return nil
	}

	// kubebuilder validation ensures the extensionRef is not nil, since disable is nil
	provider, err := fetchGatewayExtension(krtctx, *spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("extauth: %w", err)
	}
	if provider.ExtAuth == nil {
		return pluginutils.ErrInvalidExtensionType(kgateway.GatewayExtensionTypeExtAuth)
	}

	out.extAuth = &extAuthIR{
		perProviderConfig: []*perProviderExtAuthConfig{
			{
				provider:       provider,
				perRouteConfig: buildExtAuthPerRouteFilterConfig(spec),
			},
		},
		providerNames: sets.New(providerName(provider)),
	}
	return nil
}

func buildExtAuthPerRouteFilterConfig(
	spec *kgateway.ExtAuthPolicy,
) *envoy_ext_authz_v3.ExtAuthzPerRoute {
	checkSettings := &envoy_ext_authz_v3.CheckSettings{}

	if spec.WithRequestBody != nil {
		checkSettings.WithRequestBody = &envoy_ext_authz_v3.BufferSettings{
			MaxRequestBytes:     uint32(spec.WithRequestBody.MaxRequestBytes), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			AllowPartialMessage: spec.WithRequestBody.AllowPartialMessage,
			PackAsBytes:         spec.WithRequestBody.PackAsBytes,
		}
	}

	checkSettings.ContextExtensions = spec.ContextExtensions

	if proto.Size(checkSettings) > 0 {
		return &envoy_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: checkSettings,
			},
		}
	}
	return nil
}

func extAuthFilterName(name string) string {
	if name == "" {
		return extauthFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", extauthFilterNamePrefix, name)
}

// extAuthVariantFilterName returns the name of the ext-auth filter instance at
// the requested placement. The canonical name is always the pre-JWT instance;
// the post-JWT instance gets a suffix so both can coexist on one filter chain.
func extAuthVariantFilterName(providerName string, postAuthN bool) string {
	name := extAuthFilterName(providerName)
	if postAuthN {
		return name + extAuthPostAuthNSuffix
	}
	return name
}

// extAuthFilterStage preserves historical AuthN placement unless a gRPC provider
// explicitly requests metadata. Explicit metadata opt-in runs after AuthN so JWT
// metadata exists in the CheckRequest. clearRouteCache is never allowed in that
// mode by CRD validation; keep the legacy stage as a defense in depth for objects
// created before that validation was introduced.
func extAuthFilterStage(extAuth *envoy_ext_authz_v3.ExtAuthz) filters.FilterStage[filters.WellKnownFilterStage] {
	if extAuthRunsAfterAuthN(extAuth) {
		return filters.AfterStage(filters.AuthNStage)
	}
	return filters.DuringStage(filters.AuthNStage)
}

func extAuthProviderRequestsMetadata(extAuth *envoy_ext_authz_v3.ExtAuthz) bool {
	return extAuth.GetGrpcService() != nil && len(extAuth.GetMetadataContextNamespaces()) > 0
}

func extAuthRunsAfterAuthN(extAuth *envoy_ext_authz_v3.ExtAuthz) bool {
	return extAuthProviderRequestsMetadata(extAuth) && !extAuth.GetClearRouteCache()
}

func (p *trafficPolicyPluginGwPass) handleExtAuth(filterChain string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, in *extAuthIR) {
	if in == nil {
		return
	}

	// Add the global disable all filter if all providers are disabled
	if in.disableAllProviders {
		pCtxTypedFilterConfig.AddTypedConfig(ExtAuthGlobalDisableFilterName, EnableFilterPerRoute())

		// Explicitly set the ExtAuthEnabledFilter to a blank transformation.
		// This ensures that the metadata is not set if auth is not configured on the route
		AddBlankTransformationIfNeeded(pCtxTypedFilterConfig, ExtAuthEnabledFilterName, p.enableAuthMetadata)
		return
	}

	// Register the canonical placement for every provider. Post-AuthN instances
	// are registered lazily when an effective provider set actually selects one,
	// so chains with no metadata consumer keep their original filter list.
	for _, cfg := range in.perProviderConfig {
		name := providerName(cfg.provider)
		p.extAuthPerProvider.AddNamed(filterChain, extAuthVariantFilterName(name, false), name, cfg.provider, filters.DuringStage(filters.AuthNStage))
	}

	if len(in.perProviderConfig) > 0 {
		// Set the AuthSucceeded metadata field to indicate that the request has successfully been authed
		AddAuthMetadataIfNeeded(pCtxTypedFilterConfig, ExtAuthEnabledFilterName, p.enableAuthMetadata)
	}
}

// extAuthScopeState records one attachment scope's ext-auth providers. Route
// config, vhost, and route states are combined when selecting filter variants so
// inherited providers are not left behind on the legacy instance.
type extAuthScopeState struct {
	filterChain string
	providers   map[string]*perProviderExtAuthConfig
	disabled    bool
	policyRefs  []extAuthPolicyIdentity
}

type extAuthPolicyIdentity struct {
	ref        *ir.AttachedPolicyRef
	generation int64
}

func newExtAuthPolicyIdentity(ref *ir.AttachedPolicyRef, generation int64) extAuthPolicyIdentity {
	return extAuthPolicyIdentity{ref: ref, generation: generation}
}

func newExtAuthScopeState(filterChain string, in *extAuthIR, policyRefs ...extAuthPolicyIdentity) *extAuthScopeState {
	if in == nil {
		return nil
	}

	state := &extAuthScopeState{
		filterChain: filterChain,
		providers:   make(map[string]*perProviderExtAuthConfig, len(in.perProviderConfig)),
		disabled:    in.disableAllProviders,
		policyRefs:  policyRefs,
	}
	for _, cfg := range in.perProviderConfig {
		if cfg == nil || cfg.provider == nil {
			continue
		}
		state.providers[providerName(cfg.provider)] = cfg
	}
	return state
}

// extAuthSourcePolicies returns the identities of the policies that contributed
// extAuth to in, or fallback when the IR was not produced by a merge. The
// result is sorted by policy ID so finalization is deterministic.
func extAuthSourcePolicies(in *extAuthIR, fallback ...extAuthPolicyIdentity) []extAuthPolicyIdentity {
	if in == nil || len(in.sourcePolicies) == 0 {
		return fallback
	}
	ids := make([]string, 0, len(in.sourcePolicies))
	for id := range in.sourcePolicies {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]extAuthPolicyIdentity, 0, len(ids))
	for _, id := range ids {
		out = append(out, in.sourcePolicies[id])
	}
	return out
}

// recordExtAuthBackendScope records the ext-auth state for one backendRef.
// The state is stored under both the weighted entry pointer and the cluster
// name: weighted routes need the pointer so two refs to the same Service keep
// distinct contexts, while a single-backend route is looked up by cluster name
// (translateRouteAction discards the weighted entry for that case).
func (p *trafficPolicyPluginGwPass) recordExtAuthBackendScope(
	route *envoyroutev3.Route,
	weightedCluster *envoyroutev3.WeightedCluster_ClusterWeight,
	clusterName string,
	filterChain string,
	in *extAuthIR,
	policyRefs ...extAuthPolicyIdentity,
) *extAuthScopeState {
	state := newExtAuthScopeState(filterChain, in, extAuthSourcePolicies(in, policyRefs...)...)
	if state == nil {
		return nil
	}
	if route == nil {
		return state
	}
	if weightedCluster == nil && clusterName == "" {
		return state
	}
	if p.extAuthBackendStates == nil {
		p.extAuthBackendStates = map[*envoyroutev3.Route]map[any]*extAuthScopeState{}
	}
	if p.extAuthBackendStates[route] == nil {
		p.extAuthBackendStates[route] = map[any]*extAuthScopeState{}
	}
	if weightedCluster != nil {
		p.extAuthBackendStates[route][weightedCluster] = state
	}
	if clusterName != "" {
		p.extAuthBackendStates[route][clusterName] = state
	}
	return state
}

func (p *trafficPolicyPluginGwPass) recordExtAuthScope(scope any, filterChain string, in *extAuthIR, policyRefs ...extAuthPolicyIdentity) *extAuthScopeState {
	state := newExtAuthScopeState(filterChain, in, extAuthSourcePolicies(in, policyRefs...)...)
	if state == nil {
		return nil
	}

	switch v := scope.(type) {
	case *envoyroutev3.RouteConfiguration:
		if p.extAuthRouteConfigStates == nil {
			p.extAuthRouteConfigStates = map[*envoyroutev3.RouteConfiguration]*extAuthScopeState{}
		}
		p.extAuthRouteConfigStates[v] = state
	case *envoyroutev3.VirtualHost:
		if p.extAuthVhostStates == nil {
			p.extAuthVhostStates = map[*envoyroutev3.VirtualHost]*extAuthScopeState{}
		}
		p.extAuthVhostStates[v] = state
	case *envoyroutev3.Route:
		if p.extAuthRouteStates == nil {
			p.extAuthRouteStates = map[*envoyroutev3.Route]*extAuthScopeState{}
		}
		p.extAuthRouteStates[v] = state
	}
	return state
}

// extAuthBackendScopesForRoute returns the backendRef scopes of the route,
// keyed by per-backend identity (the weighted cluster pointer for weighted
// routes, or the cluster name for single-backend routes). Backend policies
// belong to an individual backendRef, so two routes that select the same
// cluster must not share them, and two weighted refs to the same Service must
// not overwrite each other.
func (p *trafficPolicyPluginGwPass) extAuthBackendScopesForRoute(route *envoyroutev3.Route) map[any]*extAuthScopeState {
	if route == nil {
		return nil
	}
	return p.extAuthBackendStates[route]
}

// extAuthScopeStates orders states from most to least specific.
func extAuthScopeStates(states ...*extAuthScopeState) []*extAuthScopeState {
	ordered := make([]*extAuthScopeState, 0, len(states))
	for _, state := range states {
		if state != nil {
			ordered = append(ordered, state)
		}
	}
	return ordered
}

func (p *trafficPolicyPluginGwPass) extAuthRouteIsReplaced(route *envoyroutev3.Route) bool {
	if route == nil {
		return true
	}
	_, replaced := p.extAuthReplacedRoutes[route]
	return replaced
}

// extAuthProvidersConflict reports whether the effective provider set combines
// a post-AuthN metadata consumer with a route-cache clearer. clearRouteCache is
// incompatible with running after JWT because the recomputed destination route
// may not have run its own authentication policies.
func extAuthProvidersConflict(providers map[string]*perProviderExtAuthConfig) bool {
	usesMetadata, clearsRouteCache := false, false
	for _, cfg := range providers {
		if cfg == nil || cfg.provider == nil || cfg.provider.ExtAuth == nil {
			continue
		}
		usesMetadata = usesMetadata || extAuthProviderRequestsMetadata(cfg.provider.ExtAuth)
		clearsRouteCache = clearsRouteCache || cfg.provider.ExtAuth.GetClearRouteCache()
	}
	return usesMetadata && clearsRouteCache
}

// reportEffectiveExtAuthConflict reports the clearRouteCache/metadata conflict
// after all attachment scopes are known, so the result does not depend on which
// hierarchy level supplied each provider.
func (p *trafficPolicyPluginGwPass) reportEffectiveExtAuthConflict(states []*extAuthScopeState) {
	if p.reporter == nil {
		return
	}
	states = extAuthScopeStates(states...)

	// Only the effective provider set matters: a provider overridden by a more
	// specific scope must not create a conflict on this route.
	providers := effectiveExtAuthProviders(states)
	if !extAuthProvidersConflict(providers) {
		return
	}

	conflicting := sets.New[*perProviderExtAuthConfig]()
	for _, cfg := range providers {
		if cfg == nil || cfg.provider == nil || cfg.provider.ExtAuth == nil {
			continue
		}
		if extAuthProviderRequestsMetadata(cfg.provider.ExtAuth) || cfg.provider.ExtAuth.GetClearRouteCache() {
			conflicting.Insert(cfg)
		}
	}

	// Attribute each conflicting provider to the policies that contributed it.
	// A policy that only set an unrelated provider in the same merged scope must
	// not be marked invalid. Merged configs carry the IDs of their source
	// policies; an unmerged config is entirely owned by its state's policies.
	// Both cases can occur together when scopes from different hierarchy levels
	// are combined, so collect reporters from every conflicting config.
	var reporters []extAuthPolicyIdentity
	for _, state := range states {
		for _, cfg := range state.providers {
			if !conflicting.Has(cfg) {
				continue
			}
			if origins := cfg.getMergeOrigins(); len(origins) > 0 {
				for _, identity := range state.policyRefs {
					if identity.ref != nil && origins.Has(identity.ref.ID()) {
						reporters = append(reporters, identity)
					}
				}
			} else {
				reporters = append(reporters, state.policyRefs...)
			}
		}
	}

	seen := sets.New[string]()
	for _, identity := range reporters {
		if identity.ref == nil {
			continue
		}
		key := identity.ref.IDWithSectionName()
		if seen.Has(key) {
			continue
		}
		seen.Insert(key)
		if p.reportedExtAuthErrors == nil {
			p.reportedExtAuthErrors = map[string]struct{}{}
		}
		if _, ok := p.reportedExtAuthErrors[key]; ok {
			continue
		}
		p.reportedExtAuthErrors[key] = struct{}{}
		p.reporter.Policy(reporter.PolicyKey{
			Group:     identity.ref.Group,
			Kind:      identity.ref.Kind,
			Namespace: identity.ref.Namespace,
			Name:      identity.ref.Name,
		}, identity.generation).AncestorRef(reporter.PolicyStatusSummaryAncestorRef()).SetCondition(reporter.PolicyCondition{
			Type:               string(shared.PolicyConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             string(shared.PolicyReasonInvalid),
			Message:            extAuthClearRouteCacheMetadataConflict,
			ObservedGeneration: identity.generation,
		})
	}
}

// extAuthRouteClusterScopes returns the ext-auth scopes for every backend the
// route can select, ordered most to least specific. A weighted route has one
// entry per weighted cluster so a backendRef policy is materialized on its own
// cluster instead of at route scope, where it would apply to its siblings.
// weightedCluster is nil for the route-level (single-cluster) case; it is the
// entry pointer for weighted routes so two refs to the same Service (same
// cluster name) still get their own config.
type extAuthRouteClusterScopes struct {
	weightedCluster *envoyroutev3.WeightedCluster_ClusterWeight
	scopes          []*extAuthScopeState
}

func extAuthScopesForRoute(
	route *envoyroutev3.Route,
	backendStates map[any]*extAuthScopeState,
	base []*extAuthScopeState,
) []extAuthRouteClusterScopes {
	action := route.GetRoute()
	if action == nil {
		return []extAuthRouteClusterScopes{{scopes: base}}
	}

	withBackend := func(key any) []*extAuthScopeState {
		state := backendStates[key]
		if state == nil {
			return base
		}
		scopes := make([]*extAuthScopeState, 0, len(base)+1)
		scopes = append(scopes, state)
		return append(scopes, base...)
	}

	if weighted := action.GetWeightedClusters().GetClusters(); len(weighted) > 0 {
		out := make([]extAuthRouteClusterScopes, 0, len(weighted))
		for _, cw := range weighted {
			out = append(out, extAuthRouteClusterScopes{
				weightedCluster: cw,
				scopes:          withBackend(cw),
			})
		}
		return out
	}

	return []extAuthRouteClusterScopes{{scopes: withBackend(action.GetCluster())}}
}

func (p *trafficPolicyPluginGwPass) materializeExtAuth(out *envoyroutev3.RouteConfiguration) {
	rcState := p.extAuthRouteConfigStates[out]

	// Register every post-AuthN instance before writing any per-filter config.
	// A legacy route can be processed before the route that opts into metadata,
	// but it still needs an explicit disable once the instance exists.
	for _, vh := range out.GetVirtualHosts() {
		vhState := p.extAuthVhostStates[vh]
		p.registerExtAuthVariantsForStates(extAuthScopeStates(vhState, rcState)...)
		for _, route := range vh.GetRoutes() {
			if p.extAuthRouteIsReplaced(route) {
				continue
			}
			routeState := p.extAuthRouteStates[route]
			backendStates := p.extAuthBackendScopesForRoute(route)
			if routeState == nil && vhState == nil && rcState == nil && len(backendStates) == 0 {
				continue
			}
			base := []*extAuthScopeState{routeState, vhState, rcState}
			for _, clusterScopes := range extAuthScopesForRoute(route, backendStates, base) {
				p.reportEffectiveExtAuthConflict(clusterScopes.scopes)
				p.registerExtAuthVariantsForStates(extAuthScopeStates(clusterScopes.scopes...)...)
			}
		}
	}

	// Finalization is authoritative for ext-auth variant entries. Provisional
	// writes from ApplyForRoute/ApplyForRouteBackend only saw a subset of the
	// attachment scopes, so a later conflict can change which variant a route
	// must use. writeExtAuthAnyConfigs below deliberately overwrites those
	// provisional selections.
	for _, vh := range out.GetVirtualHosts() {
		vhState := p.extAuthVhostStates[vh]
		vhScopes := extAuthScopeStates(vhState, rcState)
		if vhState != nil {
			vh.TypedPerFilterConfig = p.writeExtAuthAnyConfigs(vh.GetTypedPerFilterConfig(), vhScopes...)
		}
		for _, route := range vh.GetRoutes() {
			if p.extAuthRouteIsReplaced(route) {
				continue
			}
			routeState := p.extAuthRouteStates[route]
			backendStates := p.extAuthBackendScopesForRoute(route)
			if routeState == nil && vhState == nil && rcState == nil && len(backendStates) == 0 {
				continue
			}
			base := []*extAuthScopeState{routeState, vhState, rcState}
			for _, clusterScopes := range extAuthScopesForRoute(route, backendStates, base) {
				scopes := extAuthScopeStates(clusterScopes.scopes...)
				if clusterScopes.weightedCluster == nil {
					route.TypedPerFilterConfig = p.writeExtAuthAnyConfigs(route.GetTypedPerFilterConfig(), scopes...)
					continue
				}
				// Write to this exact weighted entry. Matching by cluster name
				// would apply one backendRef's context to every sibling ref to
				// the same Service.
				cw := clusterScopes.weightedCluster
				cw.TypedPerFilterConfig = p.writeExtAuthAnyConfigs(cw.GetTypedPerFilterConfig(), scopes...)
			}
		}
	}
}

func extAuthIsDisabledConfig(msg proto.Message) bool {
	fc, ok := msg.(*envoyroutev3.FilterConfig)
	return ok && fc.GetDisabled()
}

func extAuthProtoConfigIsEnabled(msg proto.Message) bool {
	if msg == nil {
		return false
	}
	if fc, ok := msg.(*envoyroutev3.FilterConfig); ok {
		return !fc.GetDisabled()
	}
	return true
}

func (p *trafficPolicyPluginGwPass) writeExtAuthProtoConfigs(dst *ir.TypedFilterConfigMap, states ...*extAuthScopeState) {
	p.writeExtAuthConfigs(func(name string, msg proto.Message) {
		// A backendRef can write an enabled config into the shared route filter
		// map. A later scope must not turn that check off while writing its own
		// unselected variant disable.
		if extAuthIsDisabledConfig(msg) && extAuthProtoConfigIsEnabled((*dst)[name]) {
			return
		}
		dst.AddTypedConfig(name, msg)
	}, states...)
}

func (p *trafficPolicyPluginGwPass) writeExtAuthAnyConfigs(dst map[string]*anypb.Any, states ...*extAuthScopeState) map[string]*anypb.Any {
	// This function is only called from finalization, after every attachment
	// scope is known. Its writes are authoritative: a provisional selection made
	// earlier by ApplyForRoute/ApplyForRouteBackend may have enabled the
	// post-AuthN variant, but the effective set can select the canonical variant
	// instead (e.g. when an inherited clearRouteCache creates a conflict). The
	// final write must be able to replace that earlier enable with a disable,
	// otherwise the route runs both instances. The guard in
	// writeExtAuthProtoConfigs exists only for the provisional per-scope writes,
	// where it protects an enabled backendRef check; it must not apply here.
	//
	// Routes that never see a metadata consumer already inherit the canonical
	// per-route config from their route/vhost/route-config scope. They only need
	// an explicit disable for post-AuthN instances that exist for other routes.
	if !extAuthStatesNeedVariantSelection(states) {
		filterChain := ""
		if len(states) > 0 {
			filterChain = states[0].filterChain
		}
		var postAuthNNames []string
		for _, provider := range p.extAuthPerProvider.Providers[filterChain] {
			if strings.HasSuffix(provider.FilterName, extAuthPostAuthNSuffix) {
				postAuthNNames = append(postAuthNNames, provider.FilterName)
			}
		}
		if len(postAuthNNames) == 0 {
			return dst
		}
		if dst == nil {
			dst = map[string]*anypb.Any{}
		}
		for _, name := range postAuthNNames {
			// Always write the disable, even if a provisional write enabled this
			// instance; finalization decides the correct selection.
			anyMsg, err := utils.MessageToAny(DisableFilterPerRoute())
			if err != nil {
				logger.Error("failed to marshal ext-auth per-route config", "filter", name, "error", err)
				continue
			}
			dst[name] = anyMsg
		}
		return dst
	}
	if dst == nil {
		dst = map[string]*anypb.Any{}
	}
	p.writeExtAuthConfigs(func(name string, msg proto.Message) {
		anyMsg, err := utils.MessageToAny(msg)
		if err != nil {
			logger.Error("failed to marshal ext-auth per-route config", "filter", name, "error", err)
			return
		}
		dst[name] = anyMsg
	}, states...)
	return dst
}

func (p *trafficPolicyPluginGwPass) writeExtAuthConfigs(set func(string, proto.Message), states ...*extAuthScopeState) {
	states = extAuthScopeStates(states...)
	if len(states) == 0 {
		return
	}

	// A disable policy uses the global disable filter written by handleExtAuth;
	// it does not need to enumerate every provider variant here.
	for _, state := range states {
		if state.disabled {
			return
		}
	}

	providers := effectiveExtAuthProviders(states)
	// A cross-scope conflict cannot be fixed by moving a subset of providers
	// after JWT. Keep every provider on the legacy pre-AuthN instance so the
	// route remains authenticated, and report the conflict on the source
	// policies. Post-AuthN instances are registered during finalization, where
	// the effective provider set of the route is known, so a partial scope
	// cannot register one that the final set must suppress.
	conflict := extAuthProvidersConflict(providers)

	for name, cfg := range providers {
		// Only a provider that explicitly opted into metadata moves after JWT.
		// Legacy providers stay before AuthN so they keep receiving the client
		// Authorization header when JWT does not forward the original token.
		usePostAuthN := !conflict && cfg != nil && cfg.provider != nil && extAuthRunsAfterAuthN(cfg.provider.ExtAuth)
		selected := extAuthVariantFilterName(name, usePostAuthN)
		unselected := extAuthVariantFilterName(name, !usePostAuthN)
		var config proto.Message
		if perRouteConfig := effectiveExtAuthPerRouteConfig(name, states); perRouteConfig != nil {
			config = perRouteConfig
		} else {
			config = EnableFilterPerRoute()
		}
		set(selected, config)
		// Do not emit config for an instance that was never registered. A later
		// materialization pass adds the disable if another route needs the
		// post-AuthN instance on this filter chain.
		if p.hasExtAuthFilter(states[0].filterChain, unselected) {
			set(unselected, DisableFilterPerRoute())
		}
	}
}

// effectiveExtAuthProviders combines providers from every attachment scope,
// with more specific scopes overriding ancestor definitions of the same provider.
func effectiveExtAuthProviders(states []*extAuthScopeState) map[string]*perProviderExtAuthConfig {
	providers := map[string]*perProviderExtAuthConfig{}
	for i := len(states) - 1; i >= 0; i-- {
		for name, cfg := range states[i].providers {
			providers[name] = cfg
		}
	}
	return providers
}

// effectiveExtAuthPerRouteConfig collapses the scopes for one filter name into
// the config Envoy would resolve for that filter. It mirrors
// FilterConfigPerRoute: context extensions are merged least to most specific,
// emit_client_span is inherited only when a more specific config does not set
// it, and every other field comes from the most specific config alone.
func effectiveExtAuthPerRouteConfig(name string, states []*extAuthScopeState) *envoy_ext_authz_v3.ExtAuthzPerRoute {
	contextExtensions := map[string]string{}
	var emitClientSpan *wrapperspb.BoolValue
	var mostSpecific *envoy_ext_authz_v3.ExtAuthzPerRoute

	for i := len(states) - 1; i >= 0; i-- {
		cfg := states[i].providers[name]
		if cfg == nil || cfg.perRouteConfig == nil {
			continue
		}
		mostSpecific = cfg.perRouteConfig
		settings, ok := cfg.perRouteConfig.GetOverride().(*envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings)
		if !ok {
			continue
		}
		for key, value := range settings.CheckSettings.GetContextExtensions() {
			contextExtensions[key] = value
		}
		if span := settings.CheckSettings.GetEmitClientSpan(); span != nil {
			emitClientSpan = proto.Clone(span).(*wrapperspb.BoolValue)
		}
	}
	if mostSpecific == nil {
		return nil
	}

	// Disabled(false) re-enables the check with the filter's own defaults. Keep
	// inherited context/span, but drop less-specific body and service settings.
	if disabled, ok := mostSpecific.GetOverride().(*envoy_ext_authz_v3.ExtAuthzPerRoute_Disabled); ok && !disabled.Disabled {
		return &envoy_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: &envoy_ext_authz_v3.CheckSettings{
					ContextExtensions: contextExtensions,
					EmitClientSpan:    emitClientSpan,
				},
			},
		}
	}

	result := proto.Clone(mostSpecific).(*envoy_ext_authz_v3.ExtAuthzPerRoute)
	if settings, ok := result.GetOverride().(*envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings); ok {
		settings.CheckSettings.ContextExtensions = contextExtensions
		settings.CheckSettings.EmitClientSpan = emitClientSpan
	}
	return result
}

// registerExtAuthVariantsForStates registers post-AuthN instances before any
// route configs are written, including for legacy routes that only need to
// explicitly disable those instances.
func (p *trafficPolicyPluginGwPass) registerExtAuthVariantsForStates(states ...*extAuthScopeState) {
	states = extAuthScopeStates(states...)
	if len(states) == 0 || !extAuthStatesNeedVariantSelection(states) {
		return
	}
	providers := effectiveExtAuthProviders(states)
	// When the effective set conflicts, every provider stays on the legacy
	// pre-AuthN instance; registering a post-AuthN variant would run the
	// metadata consumer after a route-cache clear.
	if extAuthProvidersRequestMetadata(providers) && !extAuthProvidersConflict(providers) {
		p.registerExtAuthPostVariants(states[0].filterChain, providers)
	}
}

func (p *trafficPolicyPluginGwPass) registerExtAuthPostVariants(filterChain string, providers map[string]*perProviderExtAuthConfig) {
	for name, cfg := range providers {
		if cfg == nil || cfg.provider == nil || !extAuthRunsAfterAuthN(cfg.provider.ExtAuth) {
			// A provider without metadataContextNamespaces keeps its legacy
			// pre-AuthN placement and must not be registered after JWT.
			continue
		}
		p.extAuthPerProvider.AddNamed(filterChain, extAuthVariantFilterName(name, true), name, cfg.provider, filters.AfterStage(filters.AuthNStage))
	}
}

func extAuthStatesNeedVariantSelection(states []*extAuthScopeState) bool {
	for _, state := range states {
		for _, cfg := range state.providers {
			if cfg != nil && cfg.provider != nil && extAuthRunsAfterAuthN(cfg.provider.ExtAuth) {
				return true
			}
		}
	}
	return false
}

func extAuthProvidersRequestMetadata(providers map[string]*perProviderExtAuthConfig) bool {
	for _, cfg := range providers {
		if cfg == nil || cfg.provider == nil {
			continue
		}
		if extAuthRunsAfterAuthN(cfg.provider.ExtAuth) {
			return true
		}
	}
	return false
}

func (p *trafficPolicyPluginGwPass) hasExtAuthFilter(filterChain, filterName string) bool {
	for _, provider := range p.extAuthPerProvider.Providers[filterChain] {
		if provider.FilterName == filterName {
			return true
		}
	}
	return false
}

func (c *perProviderExtAuthConfig) getMergeOrigins() sets.Set[string] {
	return c.mergeOrigins
}

func (c *perProviderExtAuthConfig) withMergeOrigins(origins sets.Set[string]) *perProviderExtAuthConfig {
	cp := *c
	cp.mergeOrigins = origins
	return &cp
}

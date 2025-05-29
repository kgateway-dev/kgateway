package routepolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"

	// TODO(nfuden): remove once rustformations are able to be used in a production environment
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/solo-io/go-utils/contextutils"
)

const (
	transformationFilterNamePrefix        = "transformation"
	extAuthGlobalDisableFilterName        = "global_disable/ext_auth"
	extAuthGlobalDisableFilterKey         = "global_disable/ext_auth"
	rustformationFilterNamePrefix         = "dynamic_modules/simple_mutations"
	metadataRouteTransformation           = "transformation/helper"
	extauthFilterNamePrefix               = "ext_auth"
	localRateLimitFilterNamePrefix        = "ratelimit/local"
	localRateLimitStatPrefix              = "http_local_rate_limiter"
	transformationFilterMetadataNamespace = "io.solo.transformation" // TODO: remove this as we move onto rustformations and off envoy-gloo
)

func extAuthFilterName(name string) string {
	if name == "" {
		return extauthFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", extauthFilterNamePrefix, name)
}

func extProcFilterName(name string) string {
	if name == "" {
		return extauthFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", "ext_proc", name)
}

type trafficPolicy struct {
	ct   time.Time
	spec trafficPolicySpecIr
}

type ExtprocIR struct {
	provider        *trafficPolicyGatewayExtensionIR
	ExtProcPerRoute *envoy_ext_proc_v3.ExtProcPerRoute
}

func (e *ExtprocIR) Equals(other *ExtprocIR) bool {
	if e == nil && other == nil {
		return true
	}
	if e == nil || other == nil {
		return false
	}

	if !proto.Equal(e.ExtProcPerRoute, other.ExtProcPerRoute) {
		return false
	}

	// Compare providers
	if e.provider == nil && other.provider == nil {
		return true
	}
	if e.provider == nil || other.provider == nil {
		return false
	}

	return e.provider.Equals(*other.provider)
}

type trafficPolicySpecIr struct {
	AI        *AIPolicyIR
	ExtProc   *ExtprocIR
	transform *transformationpb.RouteTransformations
	// rustformation is currently a *dynamicmodulesv3.DynamicModuleFilter, but can potentially change at some point
	// in the future so we use proto.Message here
	rustformation              proto.Message
	rustformationStringToStash string
	extAuth                    *extAuthIR
	localRateLimit             *localratelimitv3.LocalRateLimit
	errors                     []error
}

func (d *trafficPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *trafficPolicy) Equals(in any) bool {
	d2, ok := in.(*trafficPolicy)
	if !ok {
		return false
	}

	if d.ct != d2.ct {
		return false
	}
	if !proto.Equal(d.spec.transform, d2.spec.transform) {
		return false
	}
	if !proto.Equal(d.spec.rustformation, d2.spec.rustformation) {
		return false
	}

	// AI equality checks
	if d.spec.AI != nil && d2.spec.AI != nil {
		if d.spec.AI.AISecret != nil && d2.spec.AI.AISecret != nil && !d.spec.AI.AISecret.Equals(*d2.spec.AI.AISecret) {
			return false
		}
		if (d.spec.AI.AISecret != nil) != (d2.spec.AI.AISecret != nil) {
			return false
		}
		if !proto.Equal(d.spec.AI.Extproc, d2.spec.AI.Extproc) {
			return false
		}
		if !proto.Equal(d.spec.AI.Transformation, d2.spec.AI.Transformation) {
			return false
		}
	} else if d.spec.AI != d2.spec.AI {
		// If one of the AI IR values is nil and the other isn't, not equal
		return false
	}

	if !d.spec.extAuth.Equals(d2.spec.extAuth) {
		return false
	}

	if !d.spec.ExtProc.Equals(d2.spec.ExtProc) {
		return false
	}

	if !proto.Equal(d.spec.localRateLimit, d2.spec.localRateLimit) {
		return false
	}

	return true
}

type trafficPolicyGatewayExtensionIR struct {
	name    string
	extType v1alpha1.GatewayExtensionType

	extAuth *envoy_ext_authz_v3.ExtAuthz
	extProc *envoy_ext_proc_v3.ExternalProcessor
	err     error
}

// ResourceName returns the unique name for this extension.
func (e trafficPolicyGatewayExtensionIR) ResourceName() string {
	return e.name
}

func (e trafficPolicyGatewayExtensionIR) Equals(other trafficPolicyGatewayExtensionIR) bool {
	if e.extType != other.extType {
		return false
	}

	if !proto.Equal(e.extAuth, other.extAuth) {
		return false
	}
	if !proto.Equal(e.extProc, other.extProc) {
		return false
	}

	// Compare providers
	if e.err == nil && other.err == nil {
		return true
	}
	if e.err == nil || other.err == nil {
		return false
	}

	return e.err.Error() == other.err.Error()
}

type providerWithFromListener struct {
	provider     *trafficPolicyGatewayExtensionIR
	fromListener bool
}

type trafficPolicyPluginGwPass struct {
	setTransformationInChain bool // TODO(nfuden): make this multi stage
	// TODO(nfuden): dont abuse httplevel filter in favor of route level
	rustformationStash map[string]string
	ir.UnimplementedProxyTranslationPass
	localRateLimitInChain *localratelimitv3.LocalRateLimit
	extAuthPerProvider    map[string]providerWithFromListener
	extProcPerProvider    map[string]providerWithFromListener
}

func (p *trafficPolicyPluginGwPass) ApplyHCM(ctx context.Context, pCtx *ir.HcmContext, out *envoyhttp.HttpConnectionManager) error {
	return nil
}

var useRustformations bool

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.TrafficPolicy](
		wellknown.TrafficPolicyGVR,
		wellknown.TrafficPolicyGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().TrafficPolicies(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().TrafficPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage

	col := krt.WrapClient(kclient.New[*v1alpha1.TrafficPolicy](commoncol.Client), commoncol.KrtOpts.ToOptions("TrafficPolicy")...)
	gk := wellknown.TrafficPolicyGVK.GroupKind()

	gatewayExtensions := krt.NewCollection(commoncol.GatewayExtensions, func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *trafficPolicyGatewayExtensionIR {
		p := &trafficPolicyGatewayExtensionIR{
			name:    krt.Named{Name: gExt.Name, Namespace: gExt.Namespace}.ResourceName(),
			extType: gExt.Type,
		}

		switch gExt.Type {
		case v1alpha1.GatewayExtensionTypeExtAuth:
			envoyGrpcService, err := resolveExtGrpcService(krtctx, commoncol, gExt.ObjectSource, gExt.ExtAuth.GrpcService)
			if err != nil {
				// TODO: should this be a warning, and set cluster to blackhole?
				p.err = fmt.Errorf("failed to resolve ExtAuth backend: %w", err)
				return p
			}

			p.extAuth = &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: envoyGrpcService,
				},
				FilterEnabledMetadata: &envoy_matcher_v3.MetadataMatcher{
					Filter: transformationFilterMetadataNamespace, // the transformation filter instance's name
					Invert: true,
					Path: []*envoy_matcher_v3.MetadataMatcher_PathSegment{
						{
							Segment: &envoy_matcher_v3.MetadataMatcher_PathSegment_Key{
								Key: extAuthGlobalDisableFilterKey, // probably something like "ext-auth-enabled"
							},
						},
					},
					Value: &envoy_matcher_v3.ValueMatcher{
						MatchPattern: &envoy_matcher_v3.ValueMatcher_StringMatch{
							StringMatch: &envoy_matcher_v3.StringMatcher{
								MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
									Exact: "false",
								},
							},
						},
					},
				},
			}

		case v1alpha1.GatewayExtensionTypeExtProc:
			envoyGrpcService, err := resolveExtGrpcService(krtctx, commoncol, gExt.ObjectSource, gExt.ExtProc.GrpcService)
			if err != nil {
				p.err = fmt.Errorf("failed to resolve ExtProc backend: %w", err)
				return p
			}

			p.extProc = &envoy_ext_proc_v3.ExternalProcessor{
				GrpcService: envoyGrpcService,
			}
		}
		return p
	})

	translate := buildTranslateFunc(ctx, commoncol, gatewayExtensions)
	// TrafficPolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: policyCR.Namespace,
			Name:      policyCR.Name,
		}

		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       policyCR,
			PolicyIR:     translate(krtctx, policyCR),
			TargetRefs:   convert(policyCR.Spec.TargetRefs),
		}
		return pol
	})

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				// AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
			},
		},
		ContributesRegistration: map[schema.GroupKind]func(){
			wellknown.TrafficPolicyGVK.GroupKind(): buildRegisterCallback(ctx, commoncol.CrudClient, policyCol),
		},
		ExtraHasSynced: gatewayExtensions.HasSynced,
	}
}

func resolveExtGrpcService(krtctx krt.HandlerContext, commoncol *common.CommonCollections, objectSource ir.ObjectSource, grpcService *v1alpha1.ExtGrpcService) (*envoy_core_v3.GrpcService, error) {
	var clusterName string
	var authority string
	if grpcService != nil {
		if grpcService.BackendRef == nil {
			return nil, errors.New("backend not provided")
		}
		backendRef := grpcService.BackendRef.BackendObjectReference
		backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, objectSource, backendRef)
		if err != nil {
			return nil, err
		}
		if backend != nil {
			clusterName = backend.ClusterName()
		}
		if grpcService.Authority != nil {
			authority = *grpcService.Authority
		}
	}
	if clusterName == "" {
		return nil, errors.New("backend not found")
	}
	envoyGrpcService := &envoy_core_v3.GrpcService{
		TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
				ClusterName: clusterName,
				Authority:   authority,
			},
		},
	}
	return envoyGrpcService, nil
}

func convert(targetRefs []v1alpha1.LocalPolicyTargetReference) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Kind:  string(targetRef.Kind),
			Name:  string(targetRef.Name),
			Group: string(targetRef.Group),
		})
	}
	return refs
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &trafficPolicyPluginGwPass{}
}

func (p *trafficPolicy) Name() string {
	return "routepolicies"
}

// called 1 time for each listener
func (p *trafficPolicyPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
	policy, ok := pCtx.Policy.(*trafficPolicy)
	if !ok {
		return
	}
	if policy.spec.extAuth != nil && policy.spec.extAuth.provider != nil {
		if p.extAuthPerProvider == nil {
			p.extAuthPerProvider = make(map[string]providerWithFromListener)
		}
		k := policy.spec.extAuth.provider.ResourceName()
		p.extAuthPerProvider[k] = providerWithFromListener{
			provider:     policy.spec.extAuth.provider,
			fromListener: true,
		}
	}
	if policy.spec.ExtProc != nil && policy.spec.ExtProc.provider != nil {
		if p.extProcPerProvider == nil {
			p.extProcPerProvider = make(map[string]providerWithFromListener)
		}
		k := policy.spec.ExtProc.provider.ResourceName()
		p.extProcPerProvider[k] = providerWithFromListener{
			provider:     policy.spec.ExtProc.provider,
			fromListener: true,
		}
	}
	p.localRateLimitInChain = policy.spec.localRateLimit
}

func (p *trafficPolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *trafficPolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*trafficPolicy)
	if !ok {
		return nil
	}

	var errs []error

	if policy.spec.transform != nil {
		if policy.spec.transform != nil {
			pCtx.TypedFilterConfig.AddTypedConfig(transformationFilterNamePrefix, policy.spec.transform)
		}
		p.setTransformationInChain = true
	}

	if policy.spec.rustformation != nil {
		// TODO(nfuden): get back to this path once we have valid perroute
		// pCtx.TypedFilterConfig.AddTypedConfig(rustformationFilterNamePrefix, policy.spec.rustformation)

		// Hack around not having route level.
		// Note this is really really bad and rather fragile due to listener draining behaviors
		routeHash := strconv.Itoa(int(utils.HashProto(outputRoute)))
		if p.rustformationStash == nil {
			p.rustformationStash = make(map[string]string)
		}
		// encode the configuration that would be route level and stash the serialized version in a map
		p.rustformationStash[routeHash] = string(policy.spec.rustformationStringToStash)

		// augment the dynamic metadata so that we can do our route hack
		// set_dynamic_metadata filter DOES NOT have a route level configuration
		// set_filter_state can be used but the dynamic modules cannot access it on the current version of envoy
		// therefore use the old transformation just for rustformation
		reqm := &transformationpb.RouteTransformations_RouteTransformation_RequestMatch{
			RequestTransformation: &transformationpb.Transformation{
				TransformationType: &transformationpb.Transformation_TransformationTemplate{
					TransformationTemplate: &transformationpb.TransformationTemplate{
						ParseBodyBehavior: transformationpb.TransformationTemplate_DontParse, // Default is to try for JSON... Its kinda nice but failure is bad...
						DynamicMetadataValues: []*transformationpb.TransformationTemplate_DynamicMetadataValue{
							{
								MetadataNamespace: "kgateway",
								Key:               "route",
								Value: &transformationpb.InjaTemplate{
									Text: routeHash,
								},
							},
						},
					},
				},
			},
		}

		setmetaTransform := &transformationpb.RouteTransformations{
			Transformations: []*transformationpb.RouteTransformations_RouteTransformation{
				{
					Match: &transformationpb.RouteTransformations_RouteTransformation_RequestMatch_{
						RequestMatch: reqm,
					},
				},
			},
		}
		pCtx.TypedFilterConfig.AddTypedConfig(metadataRouteTransformation, setmetaTransform)

		p.setTransformationInChain = true
	}

	if policy.spec.localRateLimit != nil {
		pCtx.TypedFilterConfig.AddTypedConfig(localRateLimitFilterNamePrefix, policy.spec.localRateLimit)

		// Add a filter to the chain. When having a rate limit for a route we need to also have a
		// globally disabled rate limit filter in the chain otherwise it will be ignored.
		// If there is also rate limit for the listener, it will not override this one.
		if p.localRateLimitInChain == nil {
			p.localRateLimitInChain = &localratelimitv3.LocalRateLimit{
				StatPrefix: localRateLimitStatPrefix,
			}
		}
	}

	if policy.spec.AI != nil {
		var aiBackends []*v1alpha1.Backend
		// check if the backends selected by targetRef are all AI backends before applying the policy
		for _, backend := range pCtx.In.Backends {
			if backend.Backend.BackendObject == nil {
				// could be nil if not found or no ref grant
				continue
			}
			b, ok := backend.Backend.BackendObject.Obj.(*v1alpha1.Backend)
			if !ok {
				// AI policy cannot apply to kubernetes services
				// TODO(npolshak): Report this as a warning on status
				contextutils.LoggerFrom(ctx).Warnf("targetRef cannot apply to %s backend. AI TrafficPolicy must apply only to AI backend", backend.Backend.BackendObject.GetName())
				continue
			}
			if b.Spec.Type != v1alpha1.BackendTypeAI {
				// AI policy cannot apply to non-AI backends
				// TODO(npolshak): Report this as a warning on status
				contextutils.LoggerFrom(ctx).Warnf("backend %s is of type %s. AI TrafficPolicy must apply only to AI backend", backend.Backend.BackendObject.GetName(), b.Spec.Type)
				continue
			}
			aiBackends = append(aiBackends, b)
		}
		if len(aiBackends) > 0 {
			// Apply the AI policy to the all AI backends
			err := p.processAITrafficPolicy(&pCtx.TypedFilterConfig, policy.spec.AI)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	// Apply ExtAuthz configuration if present
	// ExtAuth does not allow for most information such as destination
	// to be set at the route level so we need to smuggle info upwards.
	p.handleExtAuth(&pCtx.TypedFilterConfig, policy.spec.extAuth)
	p.handleExtProc(&pCtx.TypedFilterConfig, policy.spec.ExtProc)

	return errors.Join(errs...)
}

// ApplyForBackend applies regardless if policy is attached
func (p *trafficPolicyPluginGwPass) ApplyForBackend(
	ctx context.Context,
	pCtx *ir.RouteBackendContext,
	in ir.HttpBackend,
	out *envoy_config_route_v3.Route,
) error {
	return nil
}

func (p *trafficPolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	rtPolicy, ok := policy.(*trafficPolicy)
	if !ok {
		return nil
	}

	p.handleExtAuth(&pCtx.TypedFilterConfig, rtPolicy.spec.extAuth)
	p.handleExtProc(&pCtx.TypedFilterConfig, rtPolicy.spec.ExtProc)

	if rtPolicy.spec.AI != nil && (rtPolicy.spec.AI.Transformation != nil || rtPolicy.spec.AI.Extproc != nil) {
		err := p.processAITrafficPolicy(&pCtx.TypedFilterConfig, rtPolicy.spec.AI)
		if err != nil {
			// TODO: report error on status
			contextutils.LoggerFrom(ctx).Errorf("error while processing AI TrafficPolicy: %v", err)
			return err
		}
	}

	return nil
}

func (p *trafficPolicyPluginGwPass) handleExtAuth(pCtxTypedFilterConfig *ir.TypedFilterConfigMap, extAuth *extAuthIR) {
	if extAuth == nil {
		return
	}

	// Handle the enablement state
	if extAuth.enablement == v1alpha1.ExtAuthDisableAll {
		// Disable the filter under all providers via the metadata match
		// we have to use the metadata as we dont know what other configurations may have extauth
		pCtxTypedFilterConfig.AddTypedConfig(extAuthGlobalDisableFilterName, extAuthEnablementPerRoute())
	} else {
		providerName := extAuth.provider.ResourceName()
		if extAuth.extauthPerRoute != nil {
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName),
				extAuth.extauthPerRoute,
			)
		} else if !p.extAuthPerProvider[providerName].fromListener {
			// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName),
				&envoy_ext_authz_v3.ExtAuthzPerRoute{
					Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
						CheckSettings: &envoy_ext_authz_v3.CheckSettings{},
					},
				},
			)
		}
		if p.extAuthPerProvider == nil {
			p.extAuthPerProvider = make(map[string]providerWithFromListener)
		}
		if _, ok := p.extAuthPerProvider[providerName]; !ok {
			p.extAuthPerProvider[providerName] = providerWithFromListener{
				provider: extAuth.provider,
			}
		}
	}
}

func (p *trafficPolicyPluginGwPass) handleExtProc(pCtxTypedFilterConfig *ir.TypedFilterConfigMap, extProc *ExtprocIR) {
	if extProc == nil || extProc.provider == nil {
		return
	}
	providerName := extProc.provider.ResourceName()
	// Handle the enablement state

	if extProc.ExtProcPerRoute != nil {
		pCtxTypedFilterConfig.AddTypedConfig(extProcFilterName(providerName),
			extProc.ExtProcPerRoute,
		)
	} else if !p.extProcPerProvider[providerName].fromListener {
		// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
		pCtxTypedFilterConfig.AddTypedConfig(extProcFilterName(providerName),
			&envoy_ext_proc_v3.ExtProcPerRoute{Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{Overrides: &envoy_ext_proc_v3.ExtProcOverrides{}}},
		)
	}

	if p.extProcPerProvider == nil {
		p.extProcPerProvider = make(map[string]providerWithFromListener)
	}
	if _, ok := p.extProcPerProvider[providerName]; !ok {
		p.extProcPerProvider[providerName] = providerWithFromListener{
			provider: extProc.provider,
		}
	}
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *trafficPolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	filters := []plugins.StagedHttpFilter{}

	// Add Ext_proc filters for listener
	for providerName, providerExtProc := range p.extProcPerProvider {
		extProcFilter := providerExtProc.provider.extProc
		if extProcFilter == nil {
			continue
		}

		// add the specific auth filter
		extProcName := extProcFilterName(providerName)
		stagedExtProcFilter := plugins.MustNewStagedFilter(extProcName,
			extProcFilter,
			plugins.AfterStage(plugins.WellKnownFilterStage(plugins.AuthZStage)))

		// handle the two enable attachement cases
		if !providerExtProc.fromListener {
			// handle the case where route level only should be fired
			stagedExtProcFilter.Filter.Disabled = true
		}

		filters = append(filters, stagedExtProcFilter)
	}

	if p.setTransformationInChain && !useRustformations {
		// TODO(nfuden): support stages such as early
		// first register classic
		filters = append(filters, plugins.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationpb.FilterTransformations{},
			plugins.BeforeStage(plugins.AcceptedStage)))
	}
	if p.setTransformationInChain && useRustformations {
		// ---------------
		// | END CLASSIC |
		// ---------------
		// TODO(nfuden/yuvalk): how to do route level correctly probably contribute to dynamic module upstream
		// smash together configuration
		filterRouteHashConfig := map[string]string{}

		for k, v := range p.rustformationStash {
			filterRouteHashConfig[k] = v
		}

		filterConfig, _ := json.Marshal(filterRouteHashConfig)

		rustCfg := dynamicmodulesv3.DynamicModuleFilter{
			DynamicModuleConfig: &exteniondynamicmodulev3.DynamicModuleConfig{
				Name: "rust_module",
			},
			FilterName:   "http_simple_mutations",
			FilterConfig: fmt.Sprintf(`{"route_specific": %s}`, string(filterConfig)),
		}

		filters = append(filters, plugins.MustNewStagedFilter(rustformationFilterNamePrefix,
			&rustCfg,
			plugins.BeforeStage(plugins.AcceptedStage)))

		// filters = append(filters, plugins.MustNewStagedFilter(setFilterStateFilterName,
		// 	&set_filter_statev3.Config{}, plugins.AfterStage(plugins.FaultStage)))
		filters = append(filters, plugins.MustNewStagedFilter(metadataRouteTransformation,
			&transformationpb.FilterTransformations{},
			plugins.AfterStage(plugins.FaultStage)))
	}

	// register the transformation work once
	if len(p.extAuthPerProvider) != 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		filters = append(filters, plugins.MustNewStagedFilter(extAuthGlobalDisableFilterKey,
			&transformationpb.FilterTransformations{},
			plugins.BeforeStage(plugins.FaultStage)))
	}
	// Add Ext_authz filter for listener
	for providerName, providerExtauth := range p.extAuthPerProvider {
		extAuthFilter := providerExtauth.provider.extAuth
		if extAuthFilter == nil {
			continue
		}

		// add the specific auth filter
		extauthName := extAuthFilterName(providerName)
		stagedExtAuthFilter := plugins.MustNewStagedFilter(extauthName,
			extAuthFilter,
			plugins.DuringStage(plugins.AuthZStage))

		// handle the two enable attachement cases
		if !providerExtauth.fromListener {
			// handle the case where route level only should be fired
			stagedExtAuthFilter.Filter.Disabled = true
		}

		filters = append(filters, stagedExtAuthFilter)
	}
	if p.localRateLimitInChain != nil {
		filters = append(filters, plugins.MustNewStagedFilter(localRateLimitFilterNamePrefix,
			p.localRateLimitInChain,
			plugins.BeforeStage(plugins.AcceptedStage)))
	}

	if len(filters) == 0 {
		return nil, nil
	}
	return filters, nil
}

func (p *trafficPolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *trafficPolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

func buildTranslateFunc(
	ctx context.Context,
	commoncol *common.CommonCollections, gatewayExtensions krt.Collection[trafficPolicyGatewayExtensionIR],
) func(krtctx krt.HandlerContext, i *v1alpha1.TrafficPolicy) *trafficPolicy {
	return func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) *trafficPolicy {
		policyIr := trafficPolicy{
			ct: policyCR.CreationTimestamp.Time,
		}
		outSpec := trafficPolicySpecIr{}

		if policyCR.Spec.AI != nil {
			outSpec.AI = &AIPolicyIR{}

			// Augment with AI secrets as needed
			var err error
			outSpec.AI.AISecret, err = aiSecretForSpec(ctx, commoncol.Secrets, krtctx, policyCR)
			if err != nil {
				outSpec.errors = append(outSpec.errors, err)
			}

			// Preprocess the AI backend
			err = preProcessAITrafficPolicy(policyCR.Spec.AI, outSpec.AI)
			if err != nil {
				outSpec.errors = append(outSpec.errors, err)
			}
		}
		// Apply transformation specific translation
		transformationForSpec(policyCR.Spec, &outSpec)

		if policyCR.Spec.ExtProc != nil {
			extproc, err := toEnvoyExtProc(policyCR, gatewayExtensions, krtctx, commoncol)
			if err != nil {
				outSpec.errors = append(outSpec.errors, err)
			} else {
				outSpec.ExtProc = extproc
			}
		}

		// Apply ExtAuthz specific translation
		extAuthForSpec(commoncol, krtctx, policyCR, gatewayExtensions, &outSpec)

		// Apply rate limit specific translation
		localRateLimitForSpec(policyCR.Spec, &outSpec)

		for _, err := range outSpec.errors {
			contextutils.LoggerFrom(ctx).Error(policyCR.GetNamespace(), policyCR.GetName(), err)
		}
		policyIr.spec = outSpec

		return &policyIr
	}
}

// aiSecret checks for the presence of the OpenAI Moderation which may require a secret reference
// will log an error if the secret is needed but not found
func aiSecretForSpec(
	ctx context.Context,
	secrets *krtcollections.SecretIndex,
	krtctx krt.HandlerContext,
	policyCR *v1alpha1.TrafficPolicy,
) (*ir.Secret, error) {
	if policyCR.Spec.AI == nil ||
		policyCR.Spec.AI.PromptGuard == nil ||
		policyCR.Spec.AI.PromptGuard.Request == nil ||
		policyCR.Spec.AI.PromptGuard.Request.Moderation == nil {
		return nil, nil
	}

	secretRef := policyCR.Spec.AI.PromptGuard.Request.Moderation.OpenAIModeration.AuthToken.SecretRef
	if secretRef == nil {
		// no secret ref is set
		return nil, nil
	}

	// Retrieve and assign the secret
	secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, policyCR.GetNamespace())
	if err != nil {
		contextutils.LoggerFrom(ctx).Error(err)
		return nil, err
	}
	return secret, nil
}

// transformationForSpec translates the transformation spec into and onto the IR policy
func transformationForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.Transformation == nil {
		return
	}
	var err error
	if !useRustformations {
		out.transform, err = toTransformFilterConfig(spec.Transformation)
		if err != nil {
			out.errors = append(out.errors, err)
		}
		return
	}

	rustformation, toStash, err := toRustformFilterConfig(spec.Transformation)
	if err != nil {
		out.errors = append(out.errors, err)
	}
	out.rustformation = rustformation
	out.rustformationStringToStash = toStash
}

func localRateLimitForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.RateLimit == nil || spec.RateLimit.Local == nil {
		return
	}

	var err error
	if spec.RateLimit.Local != nil {
		out.localRateLimit, err = toLocalRateLimitFilterConfig(spec.RateLimit.Local)
		if err != nil {
			// In case of an error with translating the local rate limit configuration,
			// the route will be dropped
			out.errors = append(out.errors, err)
		}
	}

	// TODO: Support rate limit extension
}

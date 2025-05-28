package trafficpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	// TODO(nfuden): remove once rustformations are able to be used in a production environment
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

const (
	transformationFilterNamePrefix              = "transformation"
	extAuthGlobalDisableFilterName              = "global_disable/ext_auth"
	extAuthGlobalDisableFilterMetadataNamespace = "dev.kgateway.disable_ext_auth"
	extAuthGlobalDisableKey                     = "extauth_disable"
	rustformationFilterNamePrefix               = "dynamic_modules/simple_mutations"
	metadataRouteTransformation                 = "transformation/helper"
	extauthFilterNamePrefix                     = "ext_auth"
	localRateLimitFilterNamePrefix              = "ratelimit/local"
	localRateLimitStatPrefix                    = "http_local_rate_limiter"
	rateLimitFilterNamePrefix                   = "ratelimit"
)

var (
	logger = logging.New("plugin/trafficpolicy")

	// from envoy code:
	// If the field `config` is configured but is empty, we treat the filter is enabled
	// explicitly.
	// see: https://github.com/envoyproxy/envoy/blob/8ed93ef372f788456b708fc93a7e54e17a013aa7/source/common/router/config_impl.cc#L2552
	EnableFilterPerRoute = &routev3.FilterConfig{Config: &anypb.Any{}}
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

func getRateLimitFilterName(name string) string {
	if name == "" {
		return rateLimitFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", rateLimitFilterNamePrefix, name)
}

type TrafficPolicy struct {
	ct   time.Time
	spec trafficPolicySpecIr
}

type ExtprocIR struct {
	provider        *TrafficPolicyGatewayExtensionIR
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
	if (e.provider == nil) != (other.provider == nil) {
		return false
	}
	if e.provider != nil && !e.provider.Equals(*other.provider) {
		return false
	}
	return true
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
	rateLimit                  *RateLimitIR
}

func (d *TrafficPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *TrafficPolicy) Equals(in any) bool {
	d2, ok := in.(*TrafficPolicy)
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

	if !d.spec.rateLimit.Equals(d2.spec.rateLimit) {
		return false
	}

	return true
}

func (r *RateLimitIR) Equals(other *RateLimitIR) bool {
	if r == nil && other == nil {
		return true
	}
	if r == nil || other == nil {
		return false
	}

	if len(r.rateLimitActions) != len(other.rateLimitActions) {
		return false
	}
	for i, action := range r.rateLimitActions {
		if !proto.Equal(action, other.rateLimitActions[i]) {
			return false
		}
	}
	if (r.provider == nil) != (other.provider == nil) {
		return false
	}
	if r.provider != nil && !r.provider.Equals(*other.provider) {
		return false
	}

	return true
}

type TrafficPolicyGatewayExtensionIR struct {
	Name      string
	ExtType   v1alpha1.GatewayExtensionType
	ExtAuth   *envoy_ext_authz_v3.ExtAuthz
	ExtProc   *envoy_ext_proc_v3.ExternalProcessor
	RateLimit *ratev3.RateLimit
	Err       error
}

// ResourceName returns the unique name for this extension.
func (e TrafficPolicyGatewayExtensionIR) ResourceName() string {
	return e.Name
}

func (e TrafficPolicyGatewayExtensionIR) Equals(other TrafficPolicyGatewayExtensionIR) bool {
	if e.ExtType != other.ExtType {
		return false
	}

	if !proto.Equal(e.ExtAuth, other.ExtAuth) {
		return false
	}
	if !proto.Equal(e.ExtProc, other.ExtProc) {
		return false
	}
	if !proto.Equal(e.RateLimit, other.RateLimit) {
		return false
	}

	// Compare providers
	if e.Err == nil && other.Err == nil {
		return true
	}
	if e.Err == nil || other.Err == nil {
		return false
	}

	return e.Err.Error() == other.Err.Error()
}

type ProviderNeededMap struct {
	// map filterhcain name -> providername -> provider
	Providers map[string]map[string]*TrafficPolicyGatewayExtensionIR
}

func (p *ProviderNeededMap) Add(fcn, providerName string, provider *TrafficPolicyGatewayExtensionIR) {
	if p.Providers == nil {
		p.Providers = make(map[string]map[string]*TrafficPolicyGatewayExtensionIR)
	}
	if p.Providers[fcn] == nil {
		p.Providers[fcn] = make(map[string]*TrafficPolicyGatewayExtensionIR)
	}
	p.Providers[fcn][providerName] = provider
}

type trafficPolicyPluginGwPass struct {
	reporter reports.Reporter
	ir.UnimplementedProxyTranslationPass

	setTransformationInChain map[string]bool // TODO(nfuden): make this multi stage
	// TODO(nfuden): dont abuse httplevel filter in favor of route level
	rustformationStash    map[string]string
	listenerTransform     *transformationpb.RouteTransformations
	localRateLimitInChain map[string]*localratelimitv3.LocalRateLimit
	extAuthPerProvider    ProviderNeededMap
	extProcPerProvider    ProviderNeededMap
	rateLimitPerProvider  ProviderNeededMap
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

func TranslateGatewayExtensionBuilder(commoncol *common.CommonCollections) func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
	return func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
		p := &TrafficPolicyGatewayExtensionIR{
			Name:    krt.Named{Name: gExt.Name, Namespace: gExt.Namespace}.ResourceName(),
			ExtType: gExt.Type,
		}

		switch gExt.Type {
		case v1alpha1.GatewayExtensionTypeExtAuth:
			envoyGrpcService, err := ResolveExtGrpcService(krtctx, commoncol.BackendIndex, false, gExt.ObjectSource, gExt.ExtAuth.GrpcService)
			if err != nil {
				// TODO: should this be a warning, and set cluster to blackhole?
				p.Err = fmt.Errorf("failed to resolve ExtAuth backend: %w", err)
				return p
			}

			p.ExtAuth = &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: envoyGrpcService,
				},
				FilterEnabledMetadata: ExtAuthzEnabledMetadataMatcher,
			}

		case v1alpha1.GatewayExtensionTypeExtProc:
			envoyGrpcService, err := ResolveExtGrpcService(krtctx, commoncol.BackendIndex, false, gExt.ObjectSource, gExt.ExtProc.GrpcService)
			if err != nil {
				p.Err = fmt.Errorf("failed to resolve ExtProc backend: %w", err)
				return p
			}

			p.ExtProc = &envoy_ext_proc_v3.ExternalProcessor{
				GrpcService: envoyGrpcService,
			}

		case v1alpha1.GatewayExtensionTypeRateLimit:
			if gExt.RateLimit == nil {
				p.Err = fmt.Errorf("rate limit extension missing configuration")
				return p
			}

			grpcService, err := ResolveExtGrpcService(krtctx, commoncol.BackendIndex, false, gExt.ObjectSource, gExt.RateLimit.GrpcService)
			if err != nil {
				p.Err = fmt.Errorf("ratelimit: %w", err)
				return p
			}

			// Use the specialized function for rate limit service resolution
			rateLimitConfig := resolveRateLimitService(grpcService, gExt.RateLimit)

			p.RateLimit = rateLimitConfig
		}
		return p
	}
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage

	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.TrafficPolicy](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("TrafficPolicy")...)
	gk := wellknown.TrafficPolicyGVK.GroupKind()

	translator := NewTrafficPolicyBuilder(ctx, commoncol)

	// TrafficPolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: policyCR.Namespace,
			Name:      policyCR.Name,
		}

		policyIR, errors := translator.Translate(krtctx, policyCR)
		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       policyCR,
			PolicyIR:     policyIR,
			TargetRefs:   pluginutils.TargetRefsToPolicyRefsWithSectionName(policyCR.Spec.TargetRefs, policyCR.Spec.TargetSelectors),
			Errors:       errors,
		}
		return pol
	})

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				// AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
				MergePolicies:             mergePolicies,
				GetPolicyStatus:           getPolicyStatusFn(commoncol.CrudClient),
				PatchPolicyStatus:         patchPolicyStatusFn(commoncol.CrudClient),
			},
		},
		ExtraHasSynced: translator.HasSynced,
	}
}

func ResolveExtGrpcService(krtctx krt.HandlerContext, backends *krtcollections.BackendIndex, disableExtensionRefValidation bool, objectSource ir.ObjectSource, grpcService *v1alpha1.ExtGrpcService) (*envoy_core_v3.GrpcService, error) {
	var clusterName string
	var authority string
	if grpcService != nil {
		if grpcService.BackendRef == nil {
			return nil, errors.New("backend not provided")
		}
		backendRef := grpcService.BackendRef.BackendObjectReference

		var backend *ir.BackendObjectIR
		var err error
		if disableExtensionRefValidation {
			backend, err = backends.GetBackendFromRefWithoutRefGrantValidation(krtctx, objectSource, backendRef)
		} else {
			backend, err = backends.GetBackendFromRef(krtctx, objectSource, backendRef)
		}
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

func resolveRateLimitService(grpcService *envoy_core_v3.GrpcService, rateLimit *v1alpha1.RateLimitProvider) *ratev3.RateLimit {
	envoyRateLimit := &ratev3.RateLimit{
		Domain:          rateLimit.Domain,
		FailureModeDeny: !rateLimit.FailOpen,
		RateLimitService: &ratelimitv3.RateLimitServiceConfig{
			GrpcService:         grpcService,
			TransportApiVersion: envoy_core_v3.ApiVersion_V3,
		},
	}

	// Set timeout if specified
	if rateLimit.Timeout != "" {
		if duration, err := time.ParseDuration(string(rateLimit.Timeout)); err == nil {
			envoyRateLimit.Timeout = durationpb.New(duration)
		} else {
			// CEL validation should catch this, so this should never happen. log it here just in case and don't error.
			logger.Error("invalid timeout in rate limit provider", "error", err)
		}
	}
	// Set defaults for other required fields
	envoyRateLimit.StatPrefix = rateLimitStatPrefix
	envoyRateLimit.EnableXRatelimitHeaders = ratev3.RateLimit_DRAFT_VERSION_03
	envoyRateLimit.RequestType = "both"

	return envoyRateLimit
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return &trafficPolicyPluginGwPass{
		reporter:                 reporter,
		setTransformationInChain: make(map[string]bool),
	}
}

func (p *TrafficPolicy) Name() string {
	return "routepolicies" // TODO: rename to trafficpolicies
}

func (p *trafficPolicyPluginGwPass) ApplyRouteConfigPlugin(ctx context.Context, pCtx *ir.RouteConfigContext, out *routev3.RouteConfiguration) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

func (p *trafficPolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *routev3.VirtualHost) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

// called 0 or more times
func (p *trafficPolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *routev3.Route) error {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return nil
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

		p.setTransformationInChain[pCtx.FilterChainName] = true
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
				logger.Warn("AI Policy cannot apply to kubernetes services", "backend_name", backend.Backend.BackendObject.GetName())
				continue
			}
			if b.Spec.Type != v1alpha1.BackendTypeAI {
				// AI policy cannot apply to non-AI backends
				// TODO(npolshak): Report this as a warning on status
				logger.Warn("AI Policy cannot apply to non-AI backend", "backend_name", backend.Backend.BackendObject.GetName(), "backend_type", string(b.Spec.Type))
				continue
			}
			aiBackends = append(aiBackends, b)
		}
		if len(aiBackends) > 0 {
			// Apply the AI policy to the all AI backends
			p.processAITrafficPolicy(&pCtx.TypedFilterConfig, policy.spec.AI)
		}
	}
	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)

	return nil
}

func (p *trafficPolicyPluginGwPass) handleTransformation(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, transform *transformationpb.RouteTransformations) {
	if transform == nil {
		return
	}

	typedFilterConfig.AddTypedConfig(transformationFilterNamePrefix, transform)
	p.setTransformationInChain[fcn] = true
}

func (p *trafficPolicyPluginGwPass) handleLocalRateLimit(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, localRateLimit *localratelimitv3.LocalRateLimit) {
	if localRateLimit == nil {
		return
	}
	typedFilterConfig.AddTypedConfig(localRateLimitFilterNamePrefix, localRateLimit)

	// Add a filter to the chain. When having a rate limit for a route we need to also have a
	// globally disabled rate limit filter in the chain otherwise it will be ignored.
	// If there is also rate limit for the listener, it will not override this one.
	if p.localRateLimitInChain == nil {
		p.localRateLimitInChain = make(map[string]*localratelimitv3.LocalRateLimit)
	}
	if _, ok := p.localRateLimitInChain[fcn]; !ok {
		p.localRateLimitInChain[fcn] = &localratelimitv3.LocalRateLimit{
			StatPrefix: localRateLimitStatPrefix,
		}
	}
}

func (p *trafficPolicyPluginGwPass) handlePolicies(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, spec trafficPolicySpecIr) {
	p.handleTransformation(fcn, typedFilterConfig, spec.transform)
	// Apply ExtAuthz configuration if present
	// ExtAuth does not allow for most information such as destination
	// to be set at the route level so we need to smuggle info upwards.
	p.handleExtAuth(fcn, typedFilterConfig, spec.extAuth)
	p.handleExtProc(fcn, typedFilterConfig, spec.ExtProc)
	// Apply rate limit configuration if present
	p.handleRateLimit(fcn, typedFilterConfig, spec.rateLimit)
	p.handleLocalRateLimit(fcn, typedFilterConfig, spec.localRateLimit)
}

// handleRateLimit adds rate limit configurations to routes
func (p *trafficPolicyPluginGwPass) handleRateLimit(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, rateLimit *RateLimitIR) {
	if rateLimit == nil {
		return
	}
	if rateLimit.rateLimitActions == nil {
		return
	}

	providerName := rateLimit.provider.ResourceName()

	// Initialize the map if it doesn't exist yet
	p.rateLimitPerProvider.Add(fcn, providerName, rateLimit.provider)

	// Configure rate limit per route - enabling it for this specific route
	rateLimitPerRoute := &ratev3.RateLimitPerRoute{
		RateLimits: rateLimit.rateLimitActions,
	}
	typedFilterConfig.AddTypedConfig(getRateLimitFilterName(providerName), rateLimitPerRoute)
}

func (p *trafficPolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	rtPolicy, ok := policy.(*TrafficPolicy)
	if !ok {
		return nil
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, rtPolicy.spec)

	if rtPolicy.spec.AI != nil && (rtPolicy.spec.AI.Transformation != nil || rtPolicy.spec.AI.Extproc != nil) {
		p.processAITrafficPolicy(&pCtx.TypedFilterConfig, rtPolicy.spec.AI)
	}

	return nil
}

func (p *trafficPolicyPluginGwPass) handleExtAuth(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, extAuth *extAuthIR) {
	if extAuth == nil {
		return
	}

	// Handle the enablement state
	if extAuth.enablement == v1alpha1.ExtAuthDisableAll {
		// Disable the filter under all providers via the metadata match
		// we have to use the metadata as we dont know what other configurations may have extauth
		pCtxTypedFilterConfig.AddTypedConfig(extAuthGlobalDisableFilterName, EnableFilterPerRoute)
	} else {
		providerName := extAuth.provider.ResourceName()
		if extAuth.extauthPerRoute != nil {
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName),
				extAuth.extauthPerRoute,
			)
		} else {
			// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName), EnableFilterPerRoute)
		}
		p.extAuthPerProvider.Add(fcn, providerName, extAuth.provider)
	}
}

func (p *trafficPolicyPluginGwPass) handleExtProc(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, extProc *ExtprocIR) {
	if extProc == nil || extProc.provider == nil {
		return
	}
	providerName := extProc.provider.ResourceName()
	// Handle the enablement state

	if extProc.ExtProcPerRoute != nil {
		pCtxTypedFilterConfig.AddTypedConfig(extProcFilterName(providerName),
			extProc.ExtProcPerRoute,
		)
	} else {
		// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
		pCtxTypedFilterConfig.AddTypedConfig(extProcFilterName(providerName),
			&envoy_ext_proc_v3.ExtProcPerRoute{Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{Overrides: &envoy_ext_proc_v3.ExtProcOverrides{}}},
		)
	}

	p.extProcPerProvider.Add(fcn, providerName, extProc.provider)
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *trafficPolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	filters := []plugins.StagedHttpFilter{}

	// Add Ext_proc filters for listener
	for providerName, provider := range p.extProcPerProvider.Providers[fcc.FilterChainName] {
		extProcFilter := provider.ExtProc
		if extProcFilter == nil {
			continue
		}

		// add the specific auth filter
		extProcName := extProcFilterName(providerName)
		stagedExtProcFilter := plugins.MustNewStagedFilter(extProcName,
			extProcFilter,
			plugins.AfterStage(plugins.WellKnownFilterStage(plugins.AuthZStage)))

		// handle the case where route level only should be fired
		stagedExtProcFilter.Filter.Disabled = true

		filters = append(filters, stagedExtProcFilter)
	}

	// register classic transforms
	if p.setTransformationInChain[fcc.FilterChainName] && !useRustformations {
		// TODO(nfuden): support stages such as early
		transformationCfg := transformationpb.FilterTransformations{}
		if p.listenerTransform != nil {
			convertClassicRouteToListener(&transformationCfg, p.listenerTransform)
		}
		filter := plugins.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationCfg,
			plugins.BeforeStage(plugins.AcceptedStage))
		filter.Filter.Disabled = true

		filters = append(filters, filter)
	}
	if p.setTransformationInChain[fcc.FilterChainName] && useRustformations {
		// ---------------
		// | END CLASSIC |
		// ---------------
		// TODO(nfuden/yuvalk): how to do route level correctly probably contribute to dynamic module upstream
		// smash together configuration
		filterRouteHashConfig := map[string]string{}
		topLevel, ok := p.rustformationStash[""]

		if topLevel == "" {
			topLevel = "}"
		} else {
			// toplevel is already formatted and at this point its quicker to rip off the { than it is so unmarshal and all}
			topLevel = "," + topLevel[1:]
		}
		if ok {
			delete(p.rustformationStash, "")
		}
		for k, v := range p.rustformationStash {
			filterRouteHashConfig[k] = v
		}

		filterConfig, _ := json.Marshal(filterRouteHashConfig)
		msg, _ := utils.MessageToAny(&wrapperspb.StringValue{
			Value: fmt.Sprintf(`{"route_specific": %s%s`, string(filterConfig), topLevel),
		})
		rustCfg := dynamicmodulesv3.DynamicModuleFilter{
			DynamicModuleConfig: &exteniondynamicmodulev3.DynamicModuleConfig{
				Name: "rust_module",
			},
			FilterName: "http_simple_mutations",

			// currently we use stringvalue but we should look at using the json variant as supported in upstream
			FilterConfig: msg,
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
	if len(p.extAuthPerProvider.Providers[fcc.FilterChainName]) != 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		filters = AddDisableFilterIfNeeded(filters)
	}

	// Add Ext_authz filter for listener
	for providerName, provider := range p.extAuthPerProvider.Providers[fcc.FilterChainName] {
		extAuthFilter := provider.ExtAuth
		if extAuthFilter == nil {
			continue
		}

		// add the specific auth filter
		extauthName := extAuthFilterName(providerName)
		stagedExtAuthFilter := plugins.MustNewStagedFilter(extauthName,
			extAuthFilter,
			plugins.DuringStage(plugins.AuthZStage))

		stagedExtAuthFilter.Filter.Disabled = true

		filters = append(filters, stagedExtAuthFilter)
	}

	if p.localRateLimitInChain[fcc.FilterChainName] != nil {
		filter := plugins.MustNewStagedFilter(localRateLimitFilterNamePrefix,
			p.localRateLimitInChain[fcc.FilterChainName],
			plugins.BeforeStage(plugins.AcceptedStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	// Add global rate limit filters from providers
	for providerName, provider := range p.rateLimitPerProvider.Providers[fcc.FilterChainName] {
		rateLimitFilter := provider.RateLimit
		if rateLimitFilter == nil {
			continue
		}

		// add the specific rate limit filter with a unique name
		rateLimitName := getRateLimitFilterName(providerName)
		stagedRateLimitFilter := plugins.MustNewStagedFilter(rateLimitName,
			rateLimitFilter,
			plugins.DuringStage(plugins.RateLimitStage))

		filters = append(filters, stagedRateLimitFilter)
	}

	if len(filters) == 0 {
		return nil, nil
	}
	return filters, nil
}

func AddDisableFilterIfNeeded(filters []plugins.StagedHttpFilter) []plugins.StagedHttpFilter {
	for _, f := range filters {
		if f.Filter.GetName() == extAuthGlobalDisableFilterName {
			return filters
		}
	}

	f := plugins.MustNewStagedFilter(extAuthGlobalDisableFilterName,
		setMetadataConfig,
		plugins.BeforeStage(plugins.FaultStage))
	f.Filter.Disabled = true
	filters = append(filters, f)
	return filters
}

func (p *trafficPolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *trafficPolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

func (p *trafficPolicyPluginGwPass) SupportsPolicyMerge() bool {
	return true
}

// mergePolicies merges the given policy ordered from high to low priority (both hierarchically
// and within the same hierarchy) based on the constraints defined per PolicyAtt.
//
// It iterates policies in reverse order (low to high) to ensure higher priority policies can
// always use an OverridableMerge strategy to override lower priority ones. Iterating policies
// in the given priority order (high to low) requires more complex merging for delegated chains
// because policies anywhere in the chain may enable policy overrides for their children but we
// still need to ensure these children cannot override any policies set by their ancestors that
// are not marked as overridable, i.e., (r1,p1)-delegate->(r2,p2)-delegate->(r3,p3) where
// r=route p=policy needs to ensure p3 does not override p1 (assuming p1 does not enable overrides)
// even if p2 allows overrides. This is easier to guarantee by using an OverridableMerge strategy
// by merging in higher priority policies with different HierarchicalPriority.
func mergePolicies(policies []ir.PolicyAtt) ir.PolicyAtt {
	var out ir.PolicyAtt
	if len(policies) == 0 {
		return out
	}
	_, ok := policies[0].PolicyIr.(*TrafficPolicy)
	// ignore unknown types
	if !ok {
		return out
	}

	// base policy to merge into has an empty PolicyIr so it can always be merged into
	out = ir.PolicyAtt{
		GroupKind:    policies[0].GroupKind,
		PolicyRef:    policies[0].PolicyRef,
		MergeOrigins: map[string]*ir.AttachedPolicyRef{},
		PolicyIr:     &TrafficPolicy{},
	}
	merged := out.PolicyIr.(*TrafficPolicy)

	for i := len(policies) - 1; i >= 0; i-- {
		mergeOpts := policy.MergeOptions{
			Strategy: policy.OverridableMerge,
		}
		// If merging a policy lower in the hierarchy with a policy higher in the hierarchy AND
		// the policy higher in the hierarchy enables policy overrides, use an AugmentedMerge strategy
		// to preserve existing fields set by lower levels.
		// NOTE: the HierarchicalPriority check is necessary to prevent enabling override behavior among
		// policies in the same hierarchy, e.g., ExtensionRef vs TargetRef policy attached to the same route, as
		// DelegationInheritedPolicyPriorityPreferChild strictly applies to parent->child policy inheritance and is not applicable
		// outside delegated policy inheritance.
		if out.HierarchicalPriority < policies[i].HierarchicalPriority && policies[i].DelegationInheritedPolicyPriority == apiannotations.DelegationInheritedPolicyPriorityPreferChild {
			mergeOpts.Strategy = policy.AugmentedMerge
		}

		p2 := policies[i].PolicyIr.(*TrafficPolicy)
		p2Ref := policies[i].PolicyRef

		if policy.IsMergeable(merged.spec.AI, p2.spec.AI, mergeOpts) {
			merged.spec.AI = p2.spec.AI
			out.MergeOrigins["ai"] = p2Ref
		}
		if policy.IsMergeable(merged.spec.ExtProc, p2.spec.ExtProc, mergeOpts) {
			merged.spec.ExtProc = p2.spec.ExtProc
			out.MergeOrigins["extProc"] = p2Ref
		}
		if policy.IsMergeable(merged.spec.transform, p2.spec.transform, mergeOpts) {
			merged.spec.transform = p2.spec.transform
			out.MergeOrigins["transformation"] = p2Ref
		}
		if policy.IsMergeable(merged.spec.rustformation, p2.spec.rustformation, mergeOpts) {
			merged.spec.rustformation = p2.spec.rustformation
			merged.spec.rustformationStringToStash = p2.spec.rustformationStringToStash
			out.MergeOrigins["rustformation"] = p2Ref
		}
		if policy.IsMergeable(merged.spec.extAuth, p2.spec.extAuth, mergeOpts) {
			merged.spec.extAuth = p2.spec.extAuth
			out.MergeOrigins["extAuth"] = p2Ref
		}
		if policy.IsMergeable(merged.spec.localRateLimit, p2.spec.localRateLimit, mergeOpts) {
			merged.spec.localRateLimit = p2.spec.localRateLimit
			out.MergeOrigins["rateLimit"] = p2Ref
		}
		// Handle global rate limit merging
		if policy.IsMergeable(merged.spec.rateLimit, p2.spec.rateLimit, mergeOpts) {
			merged.spec.rateLimit = p2.spec.rateLimit
			out.MergeOrigins["rateLimit"] = p2Ref
		}

		out.HierarchicalPriority = policies[i].HierarchicalPriority
	}

	return out
}

type TrafficPolicyBuilder struct {
	commoncol         *common.CommonCollections
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR]
	extBuilder        func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR
}

func (b *TrafficPolicyBuilder) HasSynced() bool {
	return b.gatewayExtensions.HasSynced()
}

func (b *TrafficPolicyBuilder) Translate(
	krtctx krt.HandlerContext,
	policyCR *v1alpha1.TrafficPolicy,
) (*TrafficPolicy, []error) {
	policyIr := TrafficPolicy{
		ct: policyCR.CreationTimestamp.Time,
	}
	outSpec := trafficPolicySpecIr{}

	var errors []error
	if policyCR.Spec.AI != nil {
		outSpec.AI = &AIPolicyIR{}

		// Augment with AI secrets as needed
		var err error
		outSpec.AI.AISecret, err = aiSecretForSpec(krtctx, b.commoncol.Secrets, policyCR)
		if err != nil {
			errors = append(errors, err)
		}

		// Preprocess the AI backend
		err = preProcessAITrafficPolicy(policyCR.Spec.AI, outSpec.AI)
		if err != nil {
			errors = append(errors, err)
		}
	}
	// Apply transformation specific translation
	err := transformationForSpec(policyCR.Spec, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	if policyCR.Spec.ExtProc != nil {
		extproc, err := b.toEnvoyExtProc(krtctx, policyCR)
		if err != nil {
			errors = append(errors, err)
		} else {
			outSpec.ExtProc = extproc
		}
	}

	// Apply ExtAuthz specific translation
	err = b.extAuthForSpec(krtctx, policyCR, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	// Apply rate limit specific translation
	err = localRateLimitForSpec(policyCR.Spec, &outSpec)
	if err != nil {
		errors = append(errors, err)
	}

	// Apply global rate limit specific translation
	errs := b.rateLimitForSpec(krtctx, policyCR, &outSpec)
	errors = append(errors, errs...)

	for _, err := range errors {
		logger.Error("error translating gateway extension", "namespace", policyCR.GetNamespace(), "name", policyCR.GetName(), "error", err)
	}
	policyIr.spec = outSpec

	return &policyIr, errors
}

func (b *TrafficPolicyBuilder) FetchGatewayExtension(krtctx krt.HandlerContext, extensionRef *corev1.LocalObjectReference, ns string) (*TrafficPolicyGatewayExtensionIR, error) {
	var gatewayExtension *TrafficPolicyGatewayExtensionIR
	if extensionRef != nil {
		gwExtName := types.NamespacedName{Name: extensionRef.Name, Namespace: ns}
		gatewayExtension = krt.FetchOne(krtctx, b.gatewayExtensions, krt.FilterObjectName(gwExtName))
	}
	if gatewayExtension == nil {
		return nil, fmt.Errorf("extension not found")
	}
	if gatewayExtension.Err != nil {
		return gatewayExtension, gatewayExtension.Err
	}
	return gatewayExtension, nil
}

func NewTrafficPolicyBuilder(
	ctx context.Context,
	commoncol *common.CommonCollections,
) *TrafficPolicyBuilder {
	extBuilder := TranslateGatewayExtensionBuilder(commoncol)
	defaultExtBuilder := func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
		return extBuilder(krtctx, gExt)
	}
	gatewayExtensions := krt.NewCollection(commoncol.GatewayExtensions, defaultExtBuilder)
	return &TrafficPolicyBuilder{
		commoncol:         commoncol,
		gatewayExtensions: gatewayExtensions,
		extBuilder:        extBuilder,
	}
}

// aiSecret checks for the presence of the OpenAI Moderation which may require a secret reference
// will log an error if the secret is needed but not found
func aiSecretForSpec(
	krtctx krt.HandlerContext,
	secrets *krtcollections.SecretIndex,
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
		logger.Error("failed to get secret for AI policy", "secret_name", secretRef.Name, "namespace", policyCR.GetNamespace(), "error", err)
		return nil, err
	}
	return secret, nil
}

// transformationForSpec translates the transformation spec into and onto the IR policy
func transformationForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.Transformation == nil {
		return nil
	}
	var err error
	if !useRustformations {
		out.transform, err = toTransformFilterConfig(spec.Transformation)
		if err != nil {
			return err
		}
		return nil
	}

	rustformation, toStash, err := toRustformFilterConfig(spec.Transformation)
	if err != nil {
		return err
	}
	out.rustformation = rustformation
	out.rustformationStringToStash = toStash
	return nil
}

func localRateLimitForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.RateLimit == nil || spec.RateLimit.Local == nil {
		return nil
	}

	var err error
	if spec.RateLimit.Local != nil {
		out.localRateLimit, err = toLocalRateLimitFilterConfig(spec.RateLimit.Local)
		if err != nil {
			// In case of an error with translating the local rate limit configuration,
			// the route will be dropped
			return err
		}
	}
	return nil
}

// Add this function to handle the global rate limit configuration
func (b *TrafficPolicyBuilder) rateLimitForSpec(
	krtctx krt.HandlerContext,
	policy *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
) []error {
	if policy.Spec.RateLimit == nil || policy.Spec.RateLimit.Global == nil {
		return nil
	}
	var errors []error
	globalPolicy := policy.Spec.RateLimit.Global

	// Create rate limit actions for the route or vhost
	actions, err := createRateLimitActions(globalPolicy.Descriptors)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to create rate limit actions: %w", err))
	}

	gwExtIR, err := b.FetchGatewayExtension(krtctx, globalPolicy.ExtensionRef, policy.GetNamespace())
	if err != nil {
		errors = append(errors, fmt.Errorf("ratelimit: %w", err))
		return errors
	}
	if gwExtIR.ExtType != v1alpha1.GatewayExtensionTypeRateLimit || gwExtIR.RateLimit == nil {
		errors = append(errors, pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gwExtIR.ExtType))
	}

	if len(errors) > 0 {
		return errors
	}

	// Create route rate limits and store in the RateLimitIR struct
	out.rateLimit = &RateLimitIR{
		provider: gwExtIR,
		rateLimitActions: []*routev3.RateLimit{
			{
				Actions: actions,
			},
		},
	}
	return nil
}

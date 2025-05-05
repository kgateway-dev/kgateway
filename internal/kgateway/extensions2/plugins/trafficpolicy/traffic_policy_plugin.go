package trafficpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/policy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
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
	rateLimit                  *RateLimitIR
	errors                     []error
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

	if (r.provider == nil) != (other.provider == nil) {
		return false
	}
	if r.provider != nil && other.provider != nil {
		if !r.provider.Equals(*other.provider) {
			return false
		}
	}

	if len(r.rateLimitActions) != len(other.rateLimitActions) {
		return false
	}
	for i, action := range r.rateLimitActions {
		if !proto.Equal(action, other.rateLimitActions[i]) {
			return false
		}
	}

	return true
}

type TrafficPolicyGatewayExtensionIR struct {
	name      string
	extType   v1alpha1.GatewayExtensionType
	extAuth   *envoy_ext_authz_v3.ExtAuthz
	extProc   *envoy_ext_proc_v3.ExternalProcessor
	rateLimit *ratev3.RateLimit
	err       error
}

// ResourceName returns the unique name for this extension.
func (e TrafficPolicyGatewayExtensionIR) ResourceName() string {
	return e.name
}

func (e TrafficPolicyGatewayExtensionIR) Equals(other TrafficPolicyGatewayExtensionIR) bool {
	if e.extType != other.extType {
		return false
	}

	if !proto.Equal(e.extAuth, other.extAuth) {
		return false
	}
	if !proto.Equal(e.extProc, other.extProc) {
		return false
	}
	if !proto.Equal(e.rateLimit, other.rateLimit) {
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
	provider     *TrafficPolicyGatewayExtensionIR
	fromListener bool
}

type trafficPolicyPluginGwPass struct {
	reporter reports.Reporter

	setTransformationInChain bool // TODO(nfuden): make this multi stage
	// TODO(nfuden): dont abuse httplevel filter in favor of route level
	rustformationStash map[string]string
	listenerTransform  *transformationpb.RouteTransformations
	ir.UnimplementedProxyTranslationPass
	localRateLimitInChain *localratelimitv3.LocalRateLimit
	extAuthPerProvider    map[string]providerWithFromListener
	extProcPerProvider    map[string]providerWithFromListener
	rateLimitPerProvider  map[string]providerWithFromListener
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
					Filter: extAuthGlobalDisableFilterMetadataNamespace,
					Invert: true,
					Path: []*envoy_matcher_v3.MetadataMatcher_PathSegment{
						{
							Segment: &envoy_matcher_v3.MetadataMatcher_PathSegment_Key{
								Key: extAuthGlobalDisableKey,
							},
						},
					},
					Value: &envoy_matcher_v3.ValueMatcher{
						MatchPattern: &envoy_matcher_v3.ValueMatcher_BoolMatch{
							BoolMatch: true,
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

		case v1alpha1.GatewayExtensionTypeRateLimit:
			if gExt.RateLimit == nil {
				p.err = fmt.Errorf("rate limit extension missing configuration")
				return p
			}

			// Use the specialized function for rate limit service resolution
			rateLimitConfig, err := resolveRateLimitService(krtctx, commoncol, gExt.ObjectSource, gExt.RateLimit)
			if err != nil {
				p.err = fmt.Errorf("failed to resolve RateLimit backend: %w", err)
				return p
			}

			p.rateLimit = rateLimitConfig
		}
		return p
	}
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage

	col := krt.WrapClient(kclient.New[*v1alpha1.TrafficPolicy](commoncol.Client), commoncol.KrtOpts.ToOptions("TrafficPolicy")...)
	gk := wellknown.TrafficPolicyGVK.GroupKind()

	gatewayExtensions := krt.NewCollection(commoncol.GatewayExtensions, TranslateGatewayExtensionBuilder(commoncol))

	translateFn := TrafficPolicyBuilder(ctx, commoncol, gatewayExtensions)

	// TrafficPolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: policyCR.Namespace,
			Name:      policyCR.Name,
		}

		policyIR, errors := translateFn(krtctx, policyCR)
		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       policyCR,
			PolicyIR:     policyIR,
			TargetRefs:   pluginutils.TargetRefsToPolicyRefs(policyCR.Spec.TargetRefs, policyCR.Spec.TargetSelectors),
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

func resolveRateLimitService(krtctx krt.HandlerContext, commoncol *common.CommonCollections, objectSource ir.ObjectSource, rateLimit *v1alpha1.RateLimitProvider) (*ratev3.RateLimit, error) {
	var clusterName string

	if rateLimit == nil {
		return nil, errors.New("rate limit provider not provided")
	}

	if rateLimit.GrpcService == nil {
		return nil, errors.New("grpc service not provided in rate limit provider")
	}

	if rateLimit.GrpcService.BackendRef == nil {
		return nil, errors.New("backend not provided in grpc service")
	}

	backendRef := rateLimit.GrpcService.BackendRef.BackendObjectReference
	backend, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, objectSource, backendRef)
	if err != nil {
		return nil, err
	}
	if backend != nil {
		clusterName = backend.ClusterName()
	}

	if clusterName == "" {
		return nil, errors.New("backend not found")
	}

	envoyRateLimit := &ratev3.RateLimit{
		Domain:          rateLimit.Domain,
		FailureModeDeny: !rateLimit.FailOpen,
		RateLimitService: &ratelimitv3.RateLimitServiceConfig{
			GrpcService: &envoy_core_v3.GrpcService{
				TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
						ClusterName: clusterName,
					},
				},
			},
			TransportApiVersion: envoy_core_v3.ApiVersion_V3,
		},
	}

	// Set timeout if specified
	if rateLimit.Timeout != "" {
		duration, err := time.ParseDuration(rateLimit.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout in rate limit provider: %w", err)
		}
		envoyRateLimit.Timeout = durationpb.New(duration)
	} else {
		envoyRateLimit.Timeout = durationpb.New(defaultRateLimitTimeout)
	}

	// Set defaults for other required fields
	envoyRateLimit.StatPrefix = rateLimitStatPrefix
	envoyRateLimit.EnableXRatelimitHeaders = ratev3.RateLimit_DRAFT_VERSION_03
	envoyRateLimit.RequestType = "both"

	return envoyRateLimit, nil
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return &trafficPolicyPluginGwPass{
		reporter: reporter,
	}
}

func (p *TrafficPolicy) Name() string {
	return "routepolicies" // TODO: rename to trafficpolicies
}

// called 1 time for each listener
func (p *trafficPolicyPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
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
		if p.extAuthPerProvider == nil {
			p.extAuthPerProvider = make(map[string]providerWithFromListener)
		}
		k := policy.spec.extAuth.provider.ResourceName()
		p.extAuthPerProvider[k] = providerWithFromListener{
			provider:     policy.spec.extAuth.provider,
			fromListener: true,
		}
	}
	if policy.spec.rateLimit != nil && policy.spec.rateLimit.provider != nil {
		if p.rateLimitPerProvider == nil {
			p.rateLimitPerProvider = make(map[string]providerWithFromListener)
		}
		k := policy.spec.rateLimit.provider.ResourceName()
		p.rateLimitPerProvider[k] = providerWithFromListener{
			provider:     policy.spec.rateLimit.provider,
			fromListener: true,
		}
	}

	p.localRateLimitInChain = policy.spec.localRateLimit

	if policy.spec.transform != nil {
		p.setTransformationInChain = true
		p.listenerTransform = policy.spec.transform
		if p.rustformationStash == nil {
			p.rustformationStash = make(map[string]string)
		}
		// routes cannot have an empty string id so we special case here
		p.rustformationStash[""] = policy.spec.rustformationStringToStash
	}
}

func (p *trafficPolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *routev3.VirtualHost) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	// Apply global rate limit actions to the virtual host if they exist
	if policy.spec.rateLimit != nil && policy.spec.rateLimit.rateLimitActions != nil {
		out.RateLimits = append(out.GetRateLimits(), policy.spec.rateLimit.rateLimitActions...)
	}
}

// called 0 or more times
func (p *trafficPolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *routev3.Route) error {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
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

	if policy.spec.rateLimit != nil && policy.spec.rateLimit.rateLimitActions != nil {
		// Apply global rate limit actions to the route
		route := outputRoute.GetRoute()
		if route != nil {
			route.RateLimits = append(route.GetRateLimits(), policy.spec.rateLimit.rateLimitActions...)
			// Also call handleRateLimit here to ensure proper setup in rateLimitPerProvider
			p.handleRateLimit(&pCtx.TypedFilterConfig, policy.spec.rateLimit)
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

	// Apply rate limit configuration if present
	p.handleRateLimit(&pCtx.TypedFilterConfig, policy.spec.rateLimit)

	return errors.Join(errs...)
}

// handleRateLimit adds rate limit configurations to routes
func (p *trafficPolicyPluginGwPass) handleRateLimit(pCtxTypedFilterConfig *ir.TypedFilterConfigMap, rateLimit *RateLimitIR) {
	if rateLimit == nil {
		return
	}
	if rateLimit.rateLimitActions == nil {
		return
	}

	providerName := rateLimit.provider.ResourceName()

	// Initialize the map if it doesn't exist yet
	if p.rateLimitPerProvider == nil {
		p.rateLimitPerProvider = make(map[string]providerWithFromListener)
	}
	if _, ok := p.rateLimitPerProvider[providerName]; !ok {
		p.rateLimitPerProvider[providerName] = providerWithFromListener{
			provider: rateLimit.provider,
		}
	}

	// Configure rate limit per route - enabling it for this specific route
	rateLimitPerRoute := &ratev3.RateLimitPerRoute{
		// Use the correct enum value instead of a boolean
		VhRateLimits: ratev3.RateLimitPerRoute_INCLUDE,
		RateLimits:   rateLimit.rateLimitActions,
	}
	pCtxTypedFilterConfig.AddTypedConfig(getRateLimitFilterName(providerName), rateLimitPerRoute)
}

func (p *trafficPolicyPluginGwPass) ApplyForBackend(
	ctx context.Context,
	pCtx *ir.RouteBackendContext,
	in ir.HttpBackend,
	out *routev3.Route,
) error {
	return nil
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
		pCtxTypedFilterConfig.AddTypedConfig(extAuthGlobalDisableFilterName, enableFilterPerRoute)
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

	// register classic transforms
	if p.setTransformationInChain && !useRustformations {
		// TODO(nfuden): support stages such as early
		transformationCfg := transformationpb.FilterTransformations{}
		if p.listenerTransform != nil {
			convertClassicRouteToListener(&transformationCfg, p.listenerTransform)
		}
		filters = append(filters, plugins.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationCfg,
			plugins.BeforeStage(plugins.AcceptedStage)))
	}
	if p.setTransformationInChain && useRustformations {
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
	if len(p.extAuthPerProvider) != 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		f := plugins.MustNewStagedFilter(extAuthGlobalDisableFilterName,
			setMetadataConfig,
			plugins.BeforeStage(plugins.FaultStage))
		f.Filter.Disabled = true
		filters = append(filters, f)
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

	// Add global rate limit filters from providers
	for providerName, providerRateLimit := range p.rateLimitPerProvider {
		rateLimitFilter := providerRateLimit.provider.rateLimit
		if rateLimitFilter == nil {
			continue
		}

		// add the specific rate limit filter with a unique name
		rateLimitName := getRateLimitFilterName(providerName)
		stagedRateLimitFilter := plugins.MustNewStagedFilter(rateLimitName,
			rateLimitFilter,
			plugins.DuringStage(plugins.RateLimitStage))

		// If this rate limit is not from a listener, disable it at the listener level
		// so it can be enabled selectively at the route level
		if !providerRateLimit.fromListener {
			stagedRateLimitFilter.Filter.Disabled = true
		}

		filters = append(filters, stagedRateLimitFilter)
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

func TrafficPolicyBuilder(
	ctx context.Context,
	commoncol *common.CommonCollections, gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR],
) func(krtctx krt.HandlerContext, i *v1alpha1.TrafficPolicy) (*TrafficPolicy, []error) {
	return func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) (*TrafficPolicy, []error) {
		policyIr := TrafficPolicy{
			ct: policyCR.CreationTimestamp.Time,
		}
		outSpec := trafficPolicySpecIr{}

		var errors []error
		if policyCR.Spec.AI != nil {
			outSpec.AI = &AIPolicyIR{}

			// Augment with AI secrets as needed
			var err error
			outSpec.AI.AISecret, err = aiSecretForSpec(ctx, commoncol.Secrets, krtctx, policyCR)
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
		err := transformationForSpec(ctx, policyCR.Spec, &outSpec)
		if err != nil {
			errors = append(errors, err)
		}

		if policyCR.Spec.ExtProc != nil {
			extproc, err := toEnvoyExtProc(policyCR, gatewayExtensions, krtctx, commoncol)
			if err != nil {
				errors = append(errors, err)
			} else {
				outSpec.ExtProc = extproc
			}
		}

		// Apply ExtAuthz specific translation
		err = extAuthForSpec(commoncol, krtctx, policyCR, gatewayExtensions, &outSpec)
		if err != nil {
			errors = append(errors, err)
		}

		// Apply rate limit specific translation
		err = localRateLimitForSpec(policyCR.Spec, &outSpec)
		if err != nil {
			errors = append(errors, err)
		}

		// Apply global rate limit specific translation
		rateLimitForSpec(commoncol, krtctx, policyCR, &outSpec, gatewayExtensions)

		for _, err := range errors {
			contextutils.LoggerFrom(ctx).Error(policyCR.GetNamespace(), policyCR.GetName(), err)
		}
		policyIr.spec = outSpec

		return &policyIr, errors
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
func transformationForSpec(ctx context.Context, spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.Transformation == nil {
		return nil
	}
	var err error
	if !useRustformations {
		out.transform, err = toTransformFilterConfig(ctx, spec.Transformation)
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
func rateLimitForSpec(
	commoncol *common.CommonCollections,
	krtctx krt.HandlerContext,
	policy *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR],
) {
	if policy.Spec.RateLimit == nil || policy.Spec.RateLimit.Global == nil {
		return
	}

	globalPolicy := policy.Spec.RateLimit.Global
	var provider *TrafficPolicyGatewayExtensionIR

	// Look up the GatewayExtension reference if specified
	if globalPolicy.ExtensionRef != nil {
		extensionName := string(globalPolicy.ExtensionRef.Name)
		extensionNamespace := policy.GetNamespace()

		// Find the extension in the gateway extensions collection
		named := krt.Named{Name: extensionName, Namespace: extensionNamespace}
		nsName := types.NamespacedName{Name: named.Name, Namespace: named.Namespace}
		gwExtIR := krt.FetchOne(krtctx, gatewayExtensions, krt.FilterObjectName(nsName))
		if gwExtIR == nil {
			out.errors = append(out.errors, fmt.Errorf("rate limit extension %s/%s not found",
				extensionNamespace, extensionName))
			return
		}

		if gwExtIR.extType != v1alpha1.GatewayExtensionTypeRateLimit || gwExtIR.rateLimit == nil {
			out.errors = append(out.errors,
				fmt.Errorf("extension %s/%s is not a valid rate limit extension",
					extensionNamespace, extensionName))
			return
		}

		provider = gwExtIR
	}

	// Create rate limit actions for the route or vhost
	actions, err := createRateLimitActions(globalPolicy.Descriptors)
	if err != nil {
		out.errors = append(out.errors, fmt.Errorf("failed to create rate limit actions: %w", err))
		return
	}

	// Create route rate limits and store in the RateLimitIR struct
	out.rateLimit = &RateLimitIR{
		provider: provider,
		rateLimitActions: []*routev3.RateLimit{
			{
				Actions: actions,
			},
		},
	}
}

package trafficpolicy

import (
	"errors"
	"fmt"

	xdscorev3 "github.com/cncf/xds/go/xds/core/v3"
	xdsmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	envoycompositev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/composite/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoyextprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	envoynetworkv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/matching/common_inputs/network/v3"
	envoymetadatav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/matching/input_matchers/metadata/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

type TrafficPolicyGatewayExtensionIR struct {
	Name      string
	ExtType   v1alpha1.GatewayExtensionType
	ExtAuth   *envoy_ext_authz_v3.ExtAuthz
	ExtProc   *envoymatchingv3.ExtensionWithMatcher
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

	if e.Err == nil && other.Err != nil {
		return false
	}
	if e.Err != nil && other.Err == nil {
		return false
	}
	if (e.Err != nil && other.Err != nil) && e.Err.Error() != other.Err.Error() {
		return false
	}

	return true
}

// Validate performs PGV-based validation on the gateway extension components
func (e TrafficPolicyGatewayExtensionIR) Validate() error {
	if e.Err != nil {
		// If there's an error in the IR, validation doesn't make sense.
		return nil
	}
	if e.ExtAuth != nil {
		if err := e.ExtAuth.ValidateAll(); err != nil {
			return err
		}
	}
	if e.ExtProc != nil {
		if err := e.ExtProc.ValidateAll(); err != nil {
			return err
		}
	}
	if e.RateLimit != nil {
		if err := e.RateLimit.ValidateAll(); err != nil {
			return err
		}
	}
	return nil
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
			p.ExtProc = buildCompositeExtProcFilter(envoyGrpcService)

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

func ResolveExtGrpcService(krtctx krt.HandlerContext, backends *krtcollections.BackendIndex, disableExtensionRefValidation bool, objectSource ir.ObjectSource, grpcService *v1alpha1.ExtGrpcService) (*envoycorev3.GrpcService, error) {
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
	envoyGrpcService := &envoycorev3.GrpcService{
		TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
				ClusterName: clusterName,
				Authority:   authority,
			},
		},
	}
	return envoyGrpcService, nil
}

// FIXME: Should this live here instead of the global rate limit plugin?
func resolveRateLimitService(grpcService *envoycorev3.GrpcService, rateLimit *v1alpha1.RateLimitProvider) *ratev3.RateLimit {
	envoyRateLimit := &ratev3.RateLimit{
		Domain:          rateLimit.Domain,
		FailureModeDeny: !rateLimit.FailOpen,
		RateLimitService: &envoyratelimitv3.RateLimitServiceConfig{
			GrpcService:         grpcService,
			TransportApiVersion: envoycorev3.ApiVersion_V3,
		},
	}

	// Set timeout (we expect it always to have a valid value or default due to CRD validation)
	envoyRateLimit.Timeout = durationpb.New(rateLimit.Timeout.Duration)

	// Set defaults for other required fields
	envoyRateLimit.StatPrefix = rateLimitStatPrefix
	envoyRateLimit.EnableXRatelimitHeaders = ratev3.RateLimit_DRAFT_VERSION_03
	envoyRateLimit.RequestType = "both"

	return envoyRateLimit
}

// buildCompositeExtProcFilter builds a composite filter for external processing so that
// the filter can be conditionally disabled with the global_disable/ext_proc filter is enabled
func buildCompositeExtProcFilter(envoyGrpcService *envoycorev3.GrpcService) *envoymatchingv3.ExtensionWithMatcher {
	return &envoymatchingv3.ExtensionWithMatcher{
		ExtensionConfig: &envoycorev3.TypedExtensionConfig{
			Name:        "composite_ext_proc",
			TypedConfig: utils.MustMessageToAny(&envoycompositev3.Composite{}),
		},
		XdsMatcher: &xdsmatcherv3.Matcher{
			MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
				MatcherList: &xdsmatcherv3.Matcher_MatcherList{
					Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
						{
							Predicate: &xdsmatcherv3.Matcher_MatcherList_Predicate{
								MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
									SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
										Input: &xdscorev3.TypedExtensionConfig{
											Name: globalFilterDisableMetadataKey,
											TypedConfig: utils.MustMessageToAny(&envoynetworkv3.DynamicMetadataInput{
												Filter: extProcGlobalDisableFilterMetadataNamespace,
												Path: []*envoynetworkv3.DynamicMetadataInput_PathSegment{
													{
														Segment: &envoynetworkv3.DynamicMetadataInput_PathSegment_Key{
															Key: globalFilterDisableMetadataKey,
														},
													},
												},
											}),
										},
										// This matcher succeeds when disable=true is not found in the dynamic metadata
										// for the extProcGlobalDisableFilterMetadataNamespace
										Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_CustomMatch{
											CustomMatch: &xdscorev3.TypedExtensionConfig{
												Name: "envoy.matching.matchers.metadata_matcher",
												TypedConfig: utils.MustMessageToAny(&envoymetadatav3.Metadata{
													Value: &envoymatcherv3.ValueMatcher{
														MatchPattern: &envoymatcherv3.ValueMatcher_BoolMatch{
															BoolMatch: true,
														},
													},
													Invert: true,
												}),
											},
										},
									},
								},
							},
							OnMatch: &xdsmatcherv3.Matcher_OnMatch{
								OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
									Action: &xdscorev3.TypedExtensionConfig{
										Name: "composite-action",
										TypedConfig: utils.MustMessageToAny(&envoycompositev3.ExecuteFilterAction{
											TypedConfig: &envoycorev3.TypedExtensionConfig{
												Name: "envoy.filters.http.ext_proc",
												TypedConfig: utils.MustMessageToAny(&envoyextprocv3.ExternalProcessor{
													GrpcService: envoyGrpcService,
												}),
											},
										}),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

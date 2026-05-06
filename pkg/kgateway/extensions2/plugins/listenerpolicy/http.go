package listenerpolicy

import (
	"net"
	"reflect"
	"slices"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	healthcheckv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/early_header_mutation/header_mutation/v3"
	envoyxffv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/original_ip_detection/xff/v3"
	envoyuuidv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/request_id/uuid/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

type HttpListenerPolicyIr struct {
	upgradeConfigs             []*envoy_hcm.HttpConnectionManager_UpgradeConfig
	useRemoteAddress           *bool
	xffNumTrustedHops          *uint32
	xffConfig                  *envoyxffv3.XffConfig
	skipXffAppend              *bool
	serverHeaderTransformation *envoy_hcm.HttpConnectionManager_ServerHeaderTransformation
	streamIdleTimeout          *time.Duration
	idleTimeout                *time.Duration
	healthCheckPolicy          *healthcheckv3.HealthCheck
	preserveHttp1HeaderCase    *bool
	preserveExternalRequestId  *bool
	generateRequestId          *bool
	// For a better UX, we set the default serviceName for access logs to the envoy cluster name (`<gateway-name>.<gateway-namespace>`).
	// Since the gateway name can only be determined during translation, the access log configs and policies
	// are stored so that during translation, the default serviceName is set if not already provided
	// and the final config is then marshalled.
	accessLogConfig   []proto.Message
	accessLogPolicies []kgateway.AccessLog
	// For a better UX, the default serviceName for tracing is set to the envoy cluster name (`<gateway-name>.<gateway-namespace>`).
	// Since the gateway name can only be determined during translation, the tracing config is split into the provider
	// and the actual config. During translation, the default serviceName is set if not already provided
	// and the final config is then marshalled.
	tracingProvider               *envoytracev3.OpenTelemetryConfig
	tracingConfig                 *envoy_hcm.HttpConnectionManager_Tracing
	acceptHttp10                  *bool
	defaultHostForHttp10          *string
	earlyHeaderMutationExtensions []*envoycorev3.TypedExtensionConfig
	maxRequestHeadersKb           *uint32
	uuidRequestIdConfig           *envoyuuidv3.UuidRequestIdConfig
	forwardClientCertMode         *envoy_hcm.HttpConnectionManager_ForwardClientCertDetails
	setCurrentClientCertDetails   *envoy_hcm.HttpConnectionManager_SetCurrentClientCertDetails
}

func (d *HttpListenerPolicyIr) Equals(in any) bool {
	d2, ok := in.(*HttpListenerPolicyIr)
	if !ok {
		return false
	}

	// Check the AccessLog slice
	if !slices.EqualFunc(d.accessLogConfig, d2.accessLogConfig, func(log proto.Message, log2 proto.Message) bool {
		return proto.Equal(log, log2)
	}) {
		return false
	}
	if !slices.EqualFunc(d.accessLogPolicies, d2.accessLogPolicies, func(log kgateway.AccessLog, log2 kgateway.AccessLog) bool {
		return reflect.DeepEqual(log, log2)
	}) {
		return false
	}

	// Check tracing
	if !proto.Equal(d.tracingProvider, d2.tracingProvider) {
		return false
	}
	if !proto.Equal(d.tracingConfig, d2.tracingConfig) {
		return false
	}

	// Check upgrade configs
	if !slices.EqualFunc(d.upgradeConfigs, d2.upgradeConfigs, func(cfg, cfg2 *envoy_hcm.HttpConnectionManager_UpgradeConfig) bool {
		return proto.Equal(cfg, cfg2)
	}) {
		return false
	}

	// Check useRemoteAddress
	if !cmputils.PointerValsEqual(d.useRemoteAddress, d2.useRemoteAddress) {
		return false
	}

	if !cmputils.PointerValsEqual(d.preserveExternalRequestId, d2.preserveExternalRequestId) {
		return false
	}

	if !cmputils.PointerValsEqual(d.generateRequestId, d2.generateRequestId) {
		return false
	}

	// Check xffNumTrustedHops
	if !cmputils.PointerValsEqual(d.xffNumTrustedHops, d2.xffNumTrustedHops) {
		return false
	}

	// Check xffConfig
	if !proto.Equal(d.xffConfig, d2.xffConfig) {
		return false
	}

	// Check skipXffAppend
	if !cmputils.PointerValsEqual(d.skipXffAppend, d2.skipXffAppend) {
		return false
	}

	// Check serverHeaderTransformation
	if d.serverHeaderTransformation != d2.serverHeaderTransformation {
		return false
	}

	// Check streamIdleTimeout
	if !cmputils.PointerValsEqual(d.streamIdleTimeout, d2.streamIdleTimeout) {
		return false
	}

	// Check idleTimeout
	if !cmputils.PointerValsEqual(d.idleTimeout, d2.idleTimeout) {
		return false
	}

	// Check healthCheckPolicy
	if d.healthCheckPolicy == nil && d2.healthCheckPolicy != nil {
		return false
	}
	if d.healthCheckPolicy != nil && d2.healthCheckPolicy == nil {
		return false
	}
	if d.healthCheckPolicy != nil && d2.healthCheckPolicy != nil && !proto.Equal(d.healthCheckPolicy, d2.healthCheckPolicy) {
		return false
	}

	// Check healthCheckPolicy
	if !proto.Equal(d.healthCheckPolicy, d2.healthCheckPolicy) {
		return false
	}

	if !cmputils.PointerValsEqual(d.preserveHttp1HeaderCase, d2.preserveHttp1HeaderCase) {
		return false
	}

	if !cmputils.PointerValsEqual(d.acceptHttp10, d2.acceptHttp10) {
		return false
	}

	if !cmputils.PointerValsEqual(d.defaultHostForHttp10, d2.defaultHostForHttp10) {
		return false
	}

	if !slices.EqualFunc(d.earlyHeaderMutationExtensions, d2.earlyHeaderMutationExtensions, func(a, b *envoycorev3.TypedExtensionConfig) bool {
		return proto.Equal(a, b)
	}) {
		return false
	}

	if !cmputils.PointerValsEqual(d.maxRequestHeadersKb, d2.maxRequestHeadersKb) {
		return false
	}

	if !proto.Equal(d.uuidRequestIdConfig, d2.uuidRequestIdConfig) {
		return false
	}

	if !cmputils.PointerValsEqual(d.forwardClientCertMode, d2.forwardClientCertMode) {
		return false
	}

	if !proto.Equal(d.setCurrentClientCertDetails, d2.setCurrentClientCertDetails) {
		return false
	}

	return true
}

func NewHttpListenerPolicy(krtctx krt.HandlerContext, commoncol *collections.CommonCollections, h *kgateway.HTTPSettings, objSrc ir.ObjectSource) (*HttpListenerPolicyIr, []error) {
	if h == nil {
		return nil, nil
	}
	errs := []error{}
	accessLog, err := convertAccessLogConfig(h, commoncol, krtctx, objSrc)
	if err != nil {
		logger.Error("error translating access log", "error", err)
		errs = append(errs, err)
	}

	tracingProvider, tracingConfig, err := convertTracingConfig(h, commoncol, krtctx, objSrc)
	if err != nil {
		logger.Error("error translating tracing", "error", err)
		errs = append(errs, err)
	}

	upgradeConfigs := convertUpgradeConfig(h)
	serverHeaderTransformation := convertServerHeaderTransformation(h.ServerHeaderTransformation)

	// Convert streamIdleTimeout from metav1.Duration to time.Duration
	var streamIdleTimeout *time.Duration
	if h.StreamIdleTimeout != nil {
		duration := h.StreamIdleTimeout.Duration
		streamIdleTimeout = &duration
	}

	var idleTimeout *time.Duration
	if h.IdleTimeout != nil {
		duration := h.IdleTimeout.Duration
		idleTimeout = &duration
	}

	healthCheckPolicy := convertHealthCheckPolicy(h)

	var xffNumTrustedHops *uint32
	if h.XffNumTrustedHops != nil {
		xffNumTrustedHops = new(uint32(*h.XffNumTrustedHops)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	var xffConfig *envoyxffv3.XffConfig
	// Envoy will create a default XFF Original IP Detection extension and completely ignore skipXffAppend when useRemoteAddress is false.
	// So we explicitly create the configuration here that will be able to use the configured skipXffAppend.
	if h.UseRemoteAddress != nil && !*h.UseRemoteAddress {
		xffConfig = &envoyxffv3.XffConfig{}
	}

	if xffNumTrustedHops != nil && xffConfig != nil {
		xffConfig.XffNumTrustedHops = *xffNumTrustedHops
		// Set to nil so it doesn't get added to Envoy config anymore
		xffNumTrustedHops = nil
	}

	// xffConfig should always be non-nil here since xffTrustedCIDRs may only be set if useRemoteAddress is false, but check regardless
	if xffConfig != nil && len(h.XffTrustedCIDRs) > 0 {
		var ranges []*envoycorev3.CidrRange
		for _, cidr := range h.XffTrustedCIDRs {
			ip, ipNet, err := net.ParseCIDR(string(cidr))
			if err != nil {
				logger.Error("error parsing CIDR for XFF trust", "error", err)
				errs = append(errs, err)
				continue
			}
			maskSize, _ := ipNet.Mask.Size()
			ranges = append(ranges, &envoycorev3.CidrRange{
				AddressPrefix: ip.String(),
				PrefixLen:     &wrapperspb.UInt32Value{Value: uint32(maskSize)}, // nolint:gosec // prefixLen is validated by net.ParseCIDR
			})
		}
		xffConfig.XffTrustedCidrs = &envoyxffv3.XffTrustedCidrs{Cidrs: ranges}
	}

	if xffConfig != nil && h.SkipXffAppend != nil {
		xffConfig.SkipXffAppend = &wrapperspb.BoolValue{Value: *h.SkipXffAppend}
	}

	var maxRequestHeadersKb *uint32
	if h.MaxRequestHeadersKb != nil {
		maxRequestHeadersKb = new(uint32(*h.MaxRequestHeadersKb)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	var uuidRequestIdConfig *envoyuuidv3.UuidRequestIdConfig
	if h.UuidRequestIdConfig != nil {
		uuidRequestIdConfig = &envoyuuidv3.UuidRequestIdConfig{
			PackTraceReason:              wrapperspb.Bool(ptr.Deref(h.UuidRequestIdConfig.PackTraceReason, true)),
			UseRequestIdForTraceSampling: wrapperspb.Bool(ptr.Deref(h.UuidRequestIdConfig.UseRequestIDForTraceSampling, true)),
		}
	}

	var forwardClientCertMode *envoy_hcm.HttpConnectionManager_ForwardClientCertDetails
	var setCurrentClientCertDetails *envoy_hcm.HttpConnectionManager_SetCurrentClientCertDetails
	if fccd := h.ForwardClientCertDetails; fccd != nil {
		if fccd.Mode != nil {
			switch *fccd.Mode {
			case kgateway.ForwardClientCertModeSanitize:
				forwardClientCertMode = ptr.To(envoy_hcm.HttpConnectionManager_SANITIZE)
			case kgateway.ForwardClientCertModeForwardOnly:
				forwardClientCertMode = ptr.To(envoy_hcm.HttpConnectionManager_FORWARD_ONLY)
			case kgateway.ForwardClientCertModeAppendForward:
				forwardClientCertMode = ptr.To(envoy_hcm.HttpConnectionManager_APPEND_FORWARD)
			case kgateway.ForwardClientCertModeSanitizeSet:
				forwardClientCertMode = ptr.To(envoy_hcm.HttpConnectionManager_SANITIZE_SET)
			case kgateway.ForwardClientCertModeAlwaysForwardOnly:
				forwardClientCertMode = ptr.To(envoy_hcm.HttpConnectionManager_ALWAYS_FORWARD_ONLY)
			}
		}
		if d := fccd.Details; d != nil {
			setCurrentClientCertDetails = &envoy_hcm.HttpConnectionManager_SetCurrentClientCertDetails{
				Cert:  ptr.Deref(d.Cert, false),
				Chain: ptr.Deref(d.Chain, false),
				Dns:   ptr.Deref(d.DNS, false),
				Uri:   ptr.Deref(d.URI, false),
			}
			if d.Subject != nil {
				setCurrentClientCertDetails.Subject = wrapperspb.Bool(*d.Subject)
			}
			// If Details is set but Mode is not, default to SANITIZE_SET so the
			// configuration has effect (Envoy's default SANITIZE strips XFCC).
			if forwardClientCertMode == nil {
				forwardClientCertMode = ptr.To(envoy_hcm.HttpConnectionManager_SANITIZE_SET)
			}
		}
	}

	return &HttpListenerPolicyIr{
		accessLogConfig:               accessLog,
		accessLogPolicies:             h.AccessLog,
		tracingProvider:               tracingProvider,
		tracingConfig:                 tracingConfig,
		upgradeConfigs:                upgradeConfigs,
		useRemoteAddress:              h.UseRemoteAddress,
		preserveExternalRequestId:     h.PreserveExternalRequestId,
		generateRequestId:             h.GenerateRequestId,
		xffNumTrustedHops:             xffNumTrustedHops,
		xffConfig:                     xffConfig,
		skipXffAppend:                 h.SkipXffAppend,
		serverHeaderTransformation:    serverHeaderTransformation,
		streamIdleTimeout:             streamIdleTimeout,
		idleTimeout:                   idleTimeout,
		healthCheckPolicy:             healthCheckPolicy,
		preserveHttp1HeaderCase:       h.PreserveHttp1HeaderCase,
		acceptHttp10:                  h.AcceptHttp10,
		defaultHostForHttp10:          h.DefaultHostForHttp10,
		earlyHeaderMutationExtensions: convertHeaderMutations(h.EarlyRequestHeaderModifier),
		maxRequestHeadersKb:           maxRequestHeadersKb,
		uuidRequestIdConfig:           uuidRequestIdConfig,
		forwardClientCertMode:         forwardClientCertMode,
		setCurrentClientCertDetails:   setCurrentClientCertDetails,
	}, errs
}

func convertUpgradeConfig(policy *kgateway.HTTPSettings) []*envoy_hcm.HttpConnectionManager_UpgradeConfig {
	if policy.UpgradeConfig == nil {
		return nil
	}

	configs := make([]*envoy_hcm.HttpConnectionManager_UpgradeConfig, 0, len(policy.UpgradeConfig.EnabledUpgrades))
	for _, upgradeType := range policy.UpgradeConfig.EnabledUpgrades {
		configs = append(configs, &envoy_hcm.HttpConnectionManager_UpgradeConfig{
			UpgradeType: upgradeType,
		})
	}
	return configs
}

func convertServerHeaderTransformation(transformation *kgateway.ServerHeaderTransformation) *envoy_hcm.HttpConnectionManager_ServerHeaderTransformation {
	if transformation == nil {
		return nil
	}

	switch *transformation {
	case kgateway.OverwriteServerHeaderTransformation:
		val := envoy_hcm.HttpConnectionManager_OVERWRITE
		return &val
	case kgateway.AppendIfAbsentServerHeaderTransformation:
		val := envoy_hcm.HttpConnectionManager_APPEND_IF_ABSENT
		return &val
	case kgateway.PassThroughServerHeaderTransformation:
		val := envoy_hcm.HttpConnectionManager_PASS_THROUGH
		return &val
	default:
		return nil
	}
}

func convertHealthCheckPolicy(policy *kgateway.HTTPSettings) *healthcheckv3.HealthCheck {
	if policy.HealthCheck != nil {
		return &healthcheckv3.HealthCheck{
			PassThroughMode: wrapperspb.Bool(false),
			Headers: []*envoyroutev3.HeaderMatcher{{
				Name: ":path",
				HeaderMatchSpecifier: &envoyroutev3.HeaderMatcher_StringMatch{
					StringMatch: &envoymatcherv3.StringMatcher{
						MatchPattern: &envoymatcherv3.StringMatcher_Exact{
							Exact: policy.HealthCheck.Path,
						},
					},
				},
			}},
		}
	}
	return nil
}

func convertHeaderMutations(spec *gwv1.HTTPHeaderFilter) []*envoycorev3.TypedExtensionConfig {
	mutations := pluginutils.ConvertMutations(spec)
	if len(mutations) == 0 {
		return nil
	}

	policy := &envoy_header_mutationv3.HeaderMutation{
		Mutations: mutations,
	}

	return []*envoycorev3.TypedExtensionConfig{{
		Name:        "envoy.http.early_header_mutation.header_mutation",
		TypedConfig: utils.MustMessageToAny(policy),
	}}
}

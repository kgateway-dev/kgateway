package listenerpolicy

// equality_harness_test.go verifies that HttpListenerPolicyIr.Equals detects a
// change in every field. Because all fields of HttpListenerPolicyIr are
// unexported, the harness is run with equalstest.IncludeUnexported so the
// completeness check still fails when a field is added without a matching
// Equals comparison — instead of silently dropping Envoy config updates.
//
// See test/testutils/equalstest for the harness API.

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	healthcheckv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoyxffv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/original_ip_detection/xff/v3"
	envoyuuidv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/request_id/uuid/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// baseHarnessHttpListenerPolicyIr returns a fully-populated HttpListenerPolicyIr
// so that every field can be mutated to a distinguishable value by a Case below.
func baseHarnessHttpListenerPolicyIr() *HttpListenerPolicyIr {
	return &HttpListenerPolicyIr{
		upgradeConfigs: []*envoy_hcm.HttpConnectionManager_UpgradeConfig{
			{UpgradeType: "websocket"},
		},
		useRemoteAddress:           new(true),
		xffNumTrustedHops:          new(uint32(2)),
		xffConfig:                  &envoyxffv3.XffConfig{XffNumTrustedHops: 1},
		skipXffAppend:              new(true),
		serverHeaderTransformation: new(envoy_hcm.HttpConnectionManager_OVERWRITE),
		streamIdleTimeout:          new(5 * time.Second),
		idleTimeout:                new(30 * time.Second),
		http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{
			MaxConcurrentStreams: wrapperspb.UInt32(100),
		},
		healthCheckPolicy:         &healthcheckv3.HealthCheck{PassThroughMode: wrapperspb.Bool(false)},
		preserveHttp1HeaderCase:   new(true),
		preserveExternalRequestId: new(true),
		generateRequestId:         new(true),
		accessLogConfig:           []proto.Message{wrapperspb.String("access-log")},
		accessLogPolicies: []kgateway.AccessLog{
			{FileSink: &kgateway.FileSink{Path: "/dev/stdout"}},
		},
		tracingProvider:      &envoytracev3.OpenTelemetryConfig{ServiceName: "svc"},
		tracingConfig:        &envoy_hcm.HttpConnectionManager_Tracing{MaxPathTagLength: wrapperspb.UInt32(256)},
		localReplyConfig:     &envoy_hcm.LocalReplyConfig{},
		acceptHttp10:         new(true),
		defaultHostForHttp10: new("example.com"),
		earlyHeaderMutationExtensions: []*envoycorev3.TypedExtensionConfig{
			{Name: "ext"},
		},
		maxRequestHeadersKb:      new(uint32(96)),
		maxRequestsPerConnection: new(uint32(100)),
		maxHeadersCount:          new(uint32(100)),
		uuidRequestIdConfig:      &envoyuuidv3.UuidRequestIdConfig{PackTraceReason: wrapperspb.Bool(true)},
		forwardClientCertMode:    new(envoy_hcm.HttpConnectionManager_SANITIZE_SET),
		setCurrentClientCertDetails: &envoy_hcm.HttpConnectionManager_SetCurrentClientCertDetails{
			Subject: wrapperspb.Bool(true),
		},
		stripHostPortMode: new(kgateway.StripMatchingHostPortMode),
	}
}

// TestHarnessHttpListenerPolicyIrEquals exercises every field of
// HttpListenerPolicyIr. The reflexivity check (base().Equals(base()), with two
// independent sets of pointers) also guards the serverHeaderTransformation
// regression: it compares enum values, not pointer identity.
func TestHarnessHttpListenerPolicyIrEquals(t *testing.T) {
	cases := []equalstest.Case[*HttpListenerPolicyIr]{
		{
			Field: "upgradeConfigs",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).upgradeConfigs = []*envoy_hcm.HttpConnectionManager_UpgradeConfig{{UpgradeType: "h2c"}}
			},
		},
		{
			Field:  "useRemoteAddress",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).useRemoteAddress = new(false) },
		},
		{
			Field:  "xffNumTrustedHops",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).xffNumTrustedHops = new(uint32(5)) },
		},
		{
			Field:  "xffConfig",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).xffConfig = &envoyxffv3.XffConfig{XffNumTrustedHops: 9} },
		},
		{
			Field:  "skipXffAppend",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).skipXffAppend = new(false) },
		},
		{
			Field: "serverHeaderTransformation",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).serverHeaderTransformation = new(envoy_hcm.HttpConnectionManager_APPEND_IF_ABSENT)
			},
		},
		{
			Field:  "streamIdleTimeout",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).streamIdleTimeout = new(10 * time.Second) },
		},
		{
			Field:  "idleTimeout",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).idleTimeout = new(60 * time.Second) },
		},
		{
			Field: "http2ProtocolOptions",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).http2ProtocolOptions = &envoycorev3.Http2ProtocolOptions{MaxConcurrentStreams: wrapperspb.UInt32(200)}
			},
		},
		{
			Field: "healthCheckPolicy",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).healthCheckPolicy = &healthcheckv3.HealthCheck{PassThroughMode: wrapperspb.Bool(true)}
			},
		},
		{
			Field:  "preserveHttp1HeaderCase",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).preserveHttp1HeaderCase = new(false) },
		},
		{
			Field:  "preserveExternalRequestId",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).preserveExternalRequestId = new(false) },
		},
		{
			Field:  "generateRequestId",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).generateRequestId = new(false) },
		},
		{
			Field: "accessLogConfig",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).accessLogConfig = []proto.Message{wrapperspb.String("access-log-2")}
			},
		},
		{
			Field: "accessLogPolicies",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).accessLogPolicies = []kgateway.AccessLog{{FileSink: &kgateway.FileSink{Path: "/dev/stderr"}}}
			},
		},
		{
			Field: "tracingProvider",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).tracingProvider = &envoytracev3.OpenTelemetryConfig{ServiceName: "svc-2"}
			},
		},
		{
			Field: "tracingConfig",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).tracingConfig = &envoy_hcm.HttpConnectionManager_Tracing{MaxPathTagLength: wrapperspb.UInt32(512)}
			},
		},
		{
			Field:  "localReplyConfig",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).localReplyConfig = nil },
		},
		{
			Field:  "acceptHttp10",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).acceptHttp10 = new(false) },
		},
		{
			Field:  "defaultHostForHttp10",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).defaultHostForHttp10 = new("other.com") },
		},
		{
			Field: "earlyHeaderMutationExtensions",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).earlyHeaderMutationExtensions = []*envoycorev3.TypedExtensionConfig{{Name: "ext-2"}}
			},
		},
		{
			Field:  "maxRequestHeadersKb",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).maxRequestHeadersKb = new(uint32(128)) },
		},
		{
			Field:  "maxRequestsPerConnection",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).maxRequestsPerConnection = new(uint32(200)) },
		},
		{
			Field:  "maxHeadersCount",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).maxHeadersCount = new(uint32(200)) },
		},
		{
			Field: "uuidRequestIdConfig",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).uuidRequestIdConfig = &envoyuuidv3.UuidRequestIdConfig{PackTraceReason: wrapperspb.Bool(false)}
			},
		},
		{
			Field: "forwardClientCertMode",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).forwardClientCertMode = new(envoy_hcm.HttpConnectionManager_FORWARD_ONLY)
			},
		},
		{
			Field: "setCurrentClientCertDetails",
			Mutate: func(d **HttpListenerPolicyIr) {
				(*d).setCurrentClientCertDetails = &envoy_hcm.HttpConnectionManager_SetCurrentClientCertDetails{Cert: true}
			},
		},
		{
			Field:  "stripHostPortMode",
			Mutate: func(d **HttpListenerPolicyIr) { (*d).stripHostPortMode = new(kgateway.StripAnyHostPortMode) },
		},
	}

	equalstest.Run(
		t,
		baseHarnessHttpListenerPolicyIr,
		func(a, b *HttpListenerPolicyIr) bool { return a.Equals(b) },
		cases,
		nil,
		equalstest.IncludeUnexported(),
	)
}

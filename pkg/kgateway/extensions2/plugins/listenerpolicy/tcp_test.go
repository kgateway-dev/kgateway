package listenerpolicy

import (
	"testing"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoygrpc "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoy_open_telemetry "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

func baseHarnessTcpListenerPolicyIR() *tcpListenerPolicyIR {
	return &tcpListenerPolicyIR{
		accessLogConfigs: []proto.Message{&envoyalfile.FileAccessLog{Path: "/dev/stdout"}},
		accessLogFilters: []*envoyaccesslogv3.AccessLogFilter{{
			FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_NotHealthCheckFilter{},
		}},
	}
}

func TestHarnessTcpListenerPolicyIREquals(t *testing.T) {
	cases := []equalstest.Case[*tcpListenerPolicyIR]{
		{
			Field: "accessLogConfigs",
			Mutate: func(d **tcpListenerPolicyIR) {
				(*d).accessLogConfigs = []proto.Message{&envoyalfile.FileAccessLog{Path: "/dev/stderr"}}
			},
		},
		{
			Field: "accessLogFilters",
			Mutate: func(d **tcpListenerPolicyIR) {
				(*d).accessLogFilters = []*envoyaccesslogv3.AccessLogFilter{nil}
			},
		},
	}

	equalstest.Run(
		t,
		baseHarnessTcpListenerPolicyIR,
		func(a, b *tcpListenerPolicyIR) bool { return a.Equals(b) },
		cases,
		nil,
		equalstest.IncludeUnexported(),
	)
}

func TestTcpListenerPolicyIREqualsNil(t *testing.T) {
	var nilIR *tcpListenerPolicyIR
	assert.True(t, nilIR.Equals(nil), "nil should equal nil")
	assert.False(t, nilIR.Equals(&tcpListenerPolicyIR{}), "nil should not equal an empty policy")
	assert.False(t, (&tcpListenerPolicyIR{}).Equals(nil), "an empty policy should not equal nil")
	assert.True(t, (&tcpListenerPolicyIR{}).Equals(&tcpListenerPolicyIR{}), "empty policies should be equal")
}

func TestValidateTcpAccessLog(t *testing.T) {
	cases := []struct {
		name    string
		log     kgateway.AccessLog
		wantErr string
	}{
		{
			name: "file sink without filter",
			log:  kgateway.AccessLog{FileSink: &kgateway.FileSink{Path: "/dev/stdout"}},
		},
		{
			name: "filters that apply to TCP connections",
			log: kgateway.AccessLog{
				FileSink: &kgateway.FileSink{Path: "/dev/stdout"},
				Filter: &kgateway.AccessLogFilter{
					AndFilter: []kgateway.FilterType{
						{DurationFilter: &kgateway.DurationFilter{Op: kgateway.GE, Value: 100}},
						{NotHealthCheckFilter: new(true)},
						{ResponseFlagFilter: &kgateway.ResponseFlagFilter{Flags: []string{"UF"}}},
						{CELFilter: &kgateway.CELFilter{Match: "connection.mtls"}},
					},
				},
			},
		},
		{
			name: "grpc service without additional headers",
			log: kgateway.AccessLog{GrpcService: &kgateway.AccessLogGrpcService{
				CommonAccessLogGrpcService: kgateway.CommonAccessLogGrpcService{LogName: "log"},
			}},
		},
		{
			name: "grpc service with additional request headers",
			log: kgateway.AccessLog{GrpcService: &kgateway.AccessLogGrpcService{
				CommonAccessLogGrpcService:    kgateway.CommonAccessLogGrpcService{LogName: "log"},
				AdditionalRequestHeadersToLog: []string{"x-foo"},
			}},
			wantErr: "grpcService.additionalRequestHeadersToLog, additionalResponseHeadersToLog and additionalResponseTrailersToLog are not supported for TCP access logs",
		},
		{
			name: "grpc service with additional response trailers",
			log: kgateway.AccessLog{GrpcService: &kgateway.AccessLogGrpcService{
				CommonAccessLogGrpcService:      kgateway.CommonAccessLogGrpcService{LogName: "log"},
				AdditionalResponseTrailersToLog: []string{"grpc-status"},
			}},
			wantErr: "grpcService.additionalRequestHeadersToLog, additionalResponseHeadersToLog and additionalResponseTrailersToLog are not supported for TCP access logs",
		},
		{
			name: "status code filter",
			log: kgateway.AccessLog{Filter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{StatusCodeFilter: &kgateway.StatusCodeFilter{Op: kgateway.GE, Value: 400}},
			}},
			wantErr: "filter.statusCodeFilter is not supported for TCP access logs",
		},
		{
			name: "header filter nested in andFilter",
			log: kgateway.AccessLog{Filter: &kgateway.AccessLogFilter{
				AndFilter: []kgateway.FilterType{
					{DurationFilter: &kgateway.DurationFilter{Op: kgateway.GE, Value: 100}},
					{HeaderFilter: &kgateway.HeaderFilter{}},
				},
			}},
			wantErr: "filter.headerFilter is not supported for TCP access logs",
		},
		{
			name: "grpc status filter nested in orFilter",
			log: kgateway.AccessLog{Filter: &kgateway.AccessLogFilter{
				OrFilter: []kgateway.FilterType{
					{GrpcStatusFilter: &kgateway.GrpcStatusFilter{}},
				},
			}},
			wantErr: "filter.grpcStatusFilter is not supported for TCP access logs",
		},
		{
			name: "traceable filter",
			log: kgateway.AccessLog{Filter: &kgateway.AccessLogFilter{
				FilterType: &kgateway.FilterType{TraceableFilter: new(true)},
			}},
			wantErr: "filter.traceableFilter is not supported for TCP access logs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTcpAccessLog(tc.log)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

func TestTranslateTcpProxyGrpcAccessLog(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "Backend",
		Name:      "test-service",
		Namespace: "default",
	}, 0, "", "")
	grpcService := &kgateway.AccessLogGrpcService{
		CommonAccessLogGrpcService: kgateway.CommonAccessLogGrpcService{
			CommonGrpcService: kgateway.CommonGrpcService{
				BackendRef: gwv1.BackendRef{BackendObjectReference: gwv1.BackendObjectReference{Name: "test-service"}},
			},
			LogName: "grpc-log",
		},
	}

	configs, err := translateAccessLogs(
		[]kgateway.AccessLog{{GrpcService: grpcService}},
		tcpProxyAccessLog,
		map[string]*ir.BackendObjectIR{"grpc-log-0": &backend},
	)
	require.NoError(t, err)

	want := &envoygrpc.TcpGrpcAccessLogConfig{
		CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{
			TransportApiVersion: envoycorev3.ApiVersion_V3,
			LogName:             "grpc-log",
			GrpcService: &envoycorev3.GrpcService{
				TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "backend_default_test-service_0"},
				},
			},
		},
	}
	require.Len(t, configs, 1)
	assert.True(t, proto.Equal(want, configs[0]), "gRPC sinks on TcpProxy should use the TCP gRPC access logger, got %v", configs[0])
}

func TestApplyTcpProxyAccessLogs(t *testing.T) {
	otel := &envoy_open_telemetry.OpenTelemetryAccessLogConfig{LogName: "otel"}
	tcpGrpc := &envoygrpc.TcpGrpcAccessLogConfig{
		CommonConfig: &envoygrpc.CommonGrpcAccessLogConfig{LogName: "grpc"},
	}
	filter := &envoyaccesslogv3.AccessLogFilter{
		FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_NotHealthCheckFilter{},
	}
	policyIR := &ListenerPolicyIR{
		defaultPolicy: listenerPolicy{
			tcp: &tcpListenerPolicyIR{
				accessLogConfigs: []proto.Message{tcpGrpc, otel},
				accessLogFilters: []*envoyaccesslogv3.AccessLogFilter{filter, nil},
			},
		},
	}
	gateway := func(name string) ir.GatewayIR {
		return ir.GatewayIR{SourceObject: &ir.Gateway{ObjectSource: ir.ObjectSource{Namespace: "default", Name: name}}}
	}
	pass := &listenerPolicyPluginGwPass{}

	outA := &envoytcp.TcpProxy{}
	require.NoError(t, pass.ApplyTcpProxy(&ir.TcpProxyContext{ListenerPort: 8000, Policy: policyIR, Gateway: gateway("gw-a")}, outA))
	outB := &envoytcp.TcpProxy{}
	require.NoError(t, pass.ApplyTcpProxy(&ir.TcpProxyContext{ListenerPort: 8000, Policy: policyIR, Gateway: gateway("gw-b")}, outB))

	require.Len(t, outA.GetAccessLog(), 2)
	assert.Equal(t, tcpGrpcAccessLogName, outA.GetAccessLog()[0].GetName())
	assert.True(t, proto.Equal(filter, outA.GetAccessLog()[0].GetFilter()), "filter should be carried over")
	assert.Equal(t, "envoy.access_loggers.open_telemetry", outA.GetAccessLog()[1].GetName())
	assert.Nil(t, outA.GetAccessLog()[1].GetFilter())

	assert.Equal(t, "gw-a.default", otelServiceName(t, outA.GetAccessLog()[1]))
	assert.Equal(t, "gw-b.default", otelServiceName(t, outB.GetAccessLog()[1]),
		"each gateway should get its own default resource attributes")
	assert.Nil(t, otel.GetResourceAttributes(), "the shared policy IR must not be mutated")
}

func TestApplyTcpProxyWithoutTcpSettings(t *testing.T) {
	pass := &listenerPolicyPluginGwPass{}
	out := &envoytcp.TcpProxy{}
	require.NoError(t, pass.ApplyTcpProxy(&ir.TcpProxyContext{
		ListenerPort: 8000,
		Policy:       &ListenerPolicyIR{defaultPolicy: listenerPolicy{http: &HttpListenerPolicyIr{}}},
	}, out))
	assert.Empty(t, out.GetAccessLog(), "httpSettings must not add TCP access logs")
}

func TestApplyTcpProxyPerPort(t *testing.T) {
	policyIR := &ListenerPolicyIR{
		defaultPolicy: listenerPolicy{tcp: &tcpListenerPolicyIR{
			accessLogConfigs: []proto.Message{&envoyalfile.FileAccessLog{Path: "/dev/stdout"}},
			accessLogFilters: []*envoyaccesslogv3.AccessLogFilter{nil},
		}},
		perPortPolicy: map[uint32]listenerPolicy{
			9000: {tcp: &tcpListenerPolicyIR{
				accessLogConfigs: []proto.Message{&envoyalfile.FileAccessLog{Path: "/dev/stderr"}},
				accessLogFilters: []*envoyaccesslogv3.AccessLogFilter{nil},
			}},
		},
	}
	pass := &listenerPolicyPluginGwPass{}

	for port, wantPath := range map[uint32]string{8000: "/dev/stdout", 9000: "/dev/stderr"} {
		out := &envoytcp.TcpProxy{}
		require.NoError(t, pass.ApplyTcpProxy(&ir.TcpProxyContext{ListenerPort: port, Policy: policyIR}, out))
		require.Len(t, out.GetAccessLog(), 1)
		var file envoyalfile.FileAccessLog
		require.NoError(t, out.GetAccessLog()[0].GetTypedConfig().UnmarshalTo(&file))
		assert.Equal(t, wantPath, file.GetPath(), "port %d", port)
	}
}

func TestMergeTcpSettings(t *testing.T) {
	gatewayLevel := &tcpListenerPolicyIR{accessLogConfigs: []proto.Message{&envoyalfile.FileAccessLog{Path: "/gateway"}}}
	listenerLevel := &tcpListenerPolicyIR{accessLogConfigs: []proto.Message{&envoyalfile.FileAccessLog{Path: "/listener"}}}
	p2Ref := &ir.AttachedPolicyRef{Name: "p2"}

	cases := []struct {
		name     string
		strategy policy.MergeStrategy
		p1, p2   *tcpListenerPolicyIR
		want     *tcpListenerPolicyIR
		merged   bool
	}{
		{"augmented keeps higher priority", policy.AugmentedShallowMerge, listenerLevel, gatewayLevel, listenerLevel, false},
		{"augmented fills unset", policy.AugmentedShallowMerge, nil, gatewayLevel, gatewayLevel, true},
		{"overridable replaces", policy.OverridableShallowMerge, gatewayLevel, listenerLevel, listenerLevel, true},
		{"unset p2 is a no-op", policy.OverridableShallowMerge, gatewayLevel, nil, gatewayLevel, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p1 := &listenerPolicy{tcp: tc.p1}
			mergeOrigins := ir.MergeOrigins{}
			mergeTcpSettings("default.", p1, &listenerPolicy{tcp: tc.p2}, p2Ref, nil, policy.MergeOptions{Strategy: tc.strategy}, mergeOrigins)
			assert.Same(t, tc.want, p1.tcp)
			if tc.merged {
				assert.NotEmpty(t, mergeOrigins.Get("default.tcpSettings"), "merge origin should be recorded")
			} else {
				assert.Empty(t, mergeOrigins.Get("default.tcpSettings"), "merge origin should not be recorded")
			}
		})
	}
}

func otelServiceName(t *testing.T, accessLog *envoyaccesslogv3.AccessLog) string {
	t.Helper()
	var cfg envoy_open_telemetry.OpenTelemetryAccessLogConfig
	require.NoError(t, accessLog.GetTypedConfig().UnmarshalTo(&cfg))
	for _, kv := range cfg.GetResourceAttributes().GetValues() {
		if kv.GetKey() == serviceNameKey {
			return kv.GetValue().GetStringValue()
		}
	}
	return ""
}

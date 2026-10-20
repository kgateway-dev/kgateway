package listenerpolicy

import (
	"errors"
	"fmt"
	"slices"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// tcpListenerPolicyIR is the IR of ListenerConfig.TCPSettings, applied to the Envoy TcpProxy filter.
type tcpListenerPolicyIR struct {
	// accessLogConfigs holds FileAccessLog, TcpGrpcAccessLogConfig or OpenTelemetryAccessLogConfig messages.
	// OpenTelemetry configs get their per-gateway default resource attributes during translation.
	accessLogConfigs []proto.Message
	// accessLogFilters is parallel to accessLogConfigs; an entry is nil when that access log has no filter.
	accessLogFilters []*envoyaccesslogv3.AccessLogFilter
}

func (d *tcpListenerPolicyIR) Equals(d2 *tcpListenerPolicyIR) bool {
	if d == nil || d2 == nil {
		return d == d2
	}
	if !slices.EqualFunc(d.accessLogConfigs, d2.accessLogConfigs, func(a, b proto.Message) bool {
		return proto.Equal(a, b)
	}) {
		return false
	}
	if !slices.EqualFunc(d.accessLogFilters, d2.accessLogFilters, func(a, b *envoyaccesslogv3.AccessLogFilter) bool {
		return proto.Equal(a, b)
	}) {
		return false
	}
	return true
}

func newTcpListenerPolicy(
	krtctx krt.HandlerContext,
	commoncol *collections.CommonCollections,
	t *kgateway.TCPSettings,
	objSrc ir.ObjectSource,
) (*tcpListenerPolicyIR, []error) {
	if t == nil {
		return nil, nil
	}
	configs, filters, err := convertTcpAccessLogConfig(krtctx, commoncol, t.AccessLog, objSrc)
	if err != nil {
		logger.Error("error translating tcp access log", "error", err)
		return &tcpListenerPolicyIR{}, []error{err}
	}
	return &tcpListenerPolicyIR{
		accessLogConfigs: configs,
		accessLogFilters: filters,
	}, nil
}

func convertTcpAccessLogConfig(
	krtctx krt.HandlerContext,
	commoncol *collections.CommonCollections,
	logs []kgateway.AccessLog,
	objSrc ir.ObjectSource,
) ([]proto.Message, []*envoyaccesslogv3.AccessLogFilter, error) {
	for i, log := range logs {
		if err := validateTcpAccessLog(log); err != nil {
			return nil, nil, fmt.Errorf("tcpSettings.accessLog[%d]: %w", i, err)
		}
	}

	configs, err := convertAccessLogConfig(logs, tcpProxyAccessLog, commoncol, krtctx, objSrc)
	if err != nil {
		return nil, nil, err
	}

	filters := make([]*envoyaccesslogv3.AccessLogFilter, len(configs))
	for i := range configs {
		if logs[i].Filter != nil {
			filters[i], err = convertAccessLogFilter(logs[i].Filter)
			if err != nil {
				return nil, nil, fmt.Errorf("tcpSettings.accessLog[%d]: %w", i, err)
			}
		}
	}
	return configs, filters, nil
}

// validateTcpAccessLog rejects access log fields that only make sense for HTTP traffic.
// Envoy accepts them on a TcpProxy, but they would silently match nothing (or everything).
func validateTcpAccessLog(log kgateway.AccessLog) error {
	if g := log.GrpcService; g != nil {
		if len(g.AdditionalRequestHeadersToLog) > 0 ||
			len(g.AdditionalResponseHeadersToLog) > 0 ||
			len(g.AdditionalResponseTrailersToLog) > 0 {
			return errors.New("grpcService.additionalRequestHeadersToLog, additionalResponseHeadersToLog and additionalResponseTrailersToLog are not supported for TCP access logs")
		}
	}

	f := log.Filter
	if f == nil {
		return nil
	}
	if f.FilterType != nil {
		if err := validateTcpAccessLogFilter(*f.FilterType); err != nil {
			return err
		}
	}
	for _, ft := range f.AndFilter {
		if err := validateTcpAccessLogFilter(ft); err != nil {
			return err
		}
	}
	for _, ft := range f.OrFilter {
		if err := validateTcpAccessLogFilter(ft); err != nil {
			return err
		}
	}
	return nil
}

func validateTcpAccessLogFilter(f kgateway.FilterType) error {
	var name string
	switch {
	case f.StatusCodeFilter != nil:
		name = "statusCodeFilter"
	case f.HeaderFilter != nil:
		name = "headerFilter"
	case f.GrpcStatusFilter != nil:
		name = "grpcStatusFilter"
	case f.TraceableFilter != nil:
		name = "traceableFilter"
	default:
		return nil
	}
	return fmt.Errorf("filter.%s is not supported for TCP access logs", name)
}

// AppliesToTcpFilterChains marks ListenerPolicy as applying to TCP filter chains, through
// tcpSettings and transportSocketConnectTimeout.
func (p *listenerPolicyPluginGwPass) AppliesToTcpFilterChains() {}

var _ ir.TcpFilterChainPolicyPass = &listenerPolicyPluginGwPass{}

func (p *listenerPolicyPluginGwPass) ApplyTcpProxy(
	pCtx *ir.TcpProxyContext,
	out *envoytcp.TcpProxy,
) error {
	policy := p.getPolicy(pCtx.Policy, pCtx.ListenerPort).tcp
	if policy == nil {
		return nil
	}
	for i, config := range policy.accessLogConfigs {
		if accessLog := newGatewayAccessLog(pCtx.Gateway, config, policy.accessLogFilters[i]); accessLog != nil {
			out.AccessLog = append(out.GetAccessLog(), accessLog)
		}
	}
	return nil
}

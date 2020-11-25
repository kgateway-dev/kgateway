package upstreamconn

import (
	"math"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/rotisserie/eris"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"

	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ plugins.Plugin = new(Plugin)
var _ plugins.UpstreamPlugin = new(Plugin)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {

	cfg := in.GetConnectionConfig()
	if cfg == nil {
		return nil
	}

	if cfg.MaxRequestsPerConnection > 0 {
		out.MaxRequestsPerConnection = &wrappers.UInt32Value{
			Value: cfg.MaxRequestsPerConnection,
		}
	}

	if cfg.ConnectTimeout != nil {
		out.ConnectTimeout = gogoutils.DurationStdToProto(cfg.ConnectTimeout)
	}

	if cfg.TcpKeepalive != nil {
		out.UpstreamConnectionOptions = &envoy_config_cluster_v3.UpstreamConnectionOptions{
			TcpKeepalive: convertTcpKeepAlive(cfg.TcpKeepalive),
		}
	}

	if cfg.PerConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = gogoutils.UInt32GogoToProto(cfg.PerConnectionBufferLimitBytes)
	}

	if cfg.CommonHttpProtocolOptions != nil {
		commonHttpProtocolOptions, err := convertHttpProtocolOptions(cfg.CommonHttpProtocolOptions)
		if err != nil {
			return err
		}
		out.CommonHttpProtocolOptions = commonHttpProtocolOptions
	}

	return nil
}

func convertTcpKeepAlive(tcp *v1.ConnectionConfig_TcpKeepAlive) *envoy_config_core_v3.TcpKeepalive {
	var probes *types.UInt32Value
	if tcp.KeepaliveProbes > 0 {
		probes = &types.UInt32Value{
			Value: tcp.KeepaliveProbes,
		}
	}
	return &envoy_config_core_v3.TcpKeepalive{
		KeepaliveInterval: gogoutils.UInt32GogoToProto(roundToSecond(tcp.KeepaliveInterval)),
		KeepaliveTime:     gogoutils.UInt32GogoToProto(roundToSecond(tcp.KeepaliveTime)),
		KeepaliveProbes:   gogoutils.UInt32GogoToProto(probes),
	}
}

func convertHttpProtocolOptions(hpo *v1.ConnectionConfig_HttpProtocolOptions) (*envoy_config_core_v3.HttpProtocolOptions, error) {
	out := &envoy_config_core_v3.HttpProtocolOptions{}

	if hpo.IdleTimeout != nil {
		out.IdleTimeout = gogoutils.DurationStdToProto(hpo.IdleTimeout)
	}

	if hpo.MaxHeadersCount > 0 { // Envoy requires this to be >= 1
		out.MaxHeadersCount = &wrappers.UInt32Value{Value: hpo.MaxHeadersCount}
	}

	if hpo.MaxStreamDuration != nil {
		out.MaxStreamDuration = gogoutils.DurationStdToProto(hpo.MaxStreamDuration)
	}

	switch hpo.HeadersWithUnderscoresAction {
	case v1.ConnectionConfig_HttpProtocolOptions_ALLOW:
		out.HeadersWithUnderscoresAction = envoy_config_core_v3.HttpProtocolOptions_ALLOW
	case v1.ConnectionConfig_HttpProtocolOptions_REJECT_REQUEST:
		out.HeadersWithUnderscoresAction = envoy_config_core_v3.HttpProtocolOptions_REJECT_REQUEST
	case v1.ConnectionConfig_HttpProtocolOptions_DROP_HEADER:
		out.HeadersWithUnderscoresAction = envoy_config_core_v3.HttpProtocolOptions_DROP_HEADER
	default:
		return &envoy_config_core_v3.HttpProtocolOptions{},
			eris.Errorf("invalid HeadersWithUnderscoresAction %v in CommonHttpProtocolOptions", hpo.HeadersWithUnderscoresAction)
	}

	return out, nil
}

func roundToSecond(d *time.Duration) *types.UInt32Value {
	if d == nil {
		return nil
	}

	// round up
	seconds := math.Round(d.Seconds() + 0.4999)
	return &types.UInt32Value{
		Value: uint32(seconds),
	}

}

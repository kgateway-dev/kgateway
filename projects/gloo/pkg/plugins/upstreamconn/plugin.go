package upstreamconn

import (
	"math"
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {

	cfg := in.GetConnectionConfig()
	if cfg == nil {
		return nil
	}

	if cfg.MaxRequestsPerConnection > 0 {
		out.MaxRequestsPerConnection = &types.UInt32Value{
			Value: cfg.MaxRequestsPerConnection,
		}
	}

	if cfg.ConnectTimeout != nil {
		out.ConnectTimeout = *cfg.ConnectTimeout
	}

	if cfg.TcpKeepalive != nil {
		out.UpstreamConnectionOptions = &envoyapi.UpstreamConnectionOptions{
			TcpKeepalive: convertTcpKeepAlive(cfg.TcpKeepalive),
		}
	}

	return nil
}

func convertTcpKeepAlive(tcp *v1.ConnectionConfig_TcpKeepAlive) *envoycore.TcpKeepalive {
	var probes *types.UInt32Value
	if tcp.KeepaliveProbes > 0 {
		probes = &types.UInt32Value{
			Value: tcp.KeepaliveProbes,
		}
	}
	return &envoycore.TcpKeepalive{
		KeepaliveInterval: roundToSecond(tcp.KeepaliveInterval),
		KeepaliveTime:     roundToSecond(tcp.KeepaliveTime),
		KeepaliveProbes:   probes,
	}
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

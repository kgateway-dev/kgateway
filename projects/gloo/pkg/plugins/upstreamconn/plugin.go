package upstreamconn

import (
	"math"
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"

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
		out.MaxRequestsPerConnection = &wrappers.UInt32Value{
			Value: cfg.MaxRequestsPerConnection,
		}
	}

	if cfg.ConnectTimeout != nil {
		out.ConnectTimeout = gogoutils.DurationStdToProto(cfg.ConnectTimeout)
	}

	if cfg.TcpKeepalive != nil {
		out.UpstreamConnectionOptions = &envoyapi.UpstreamConnectionOptions{
			TcpKeepalive: convertTcpKeepAlive(cfg.TcpKeepalive),
		}
	}

	if cfg.PerConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = gogoutils.UInt32GogoToProto(cfg.PerConnectionBufferLimitBytes)
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
		KeepaliveInterval: gogoutils.UInt32GogoToProto(roundToSecond(tcp.KeepaliveInterval)),
		KeepaliveTime:     gogoutils.UInt32GogoToProto(roundToSecond(tcp.KeepaliveTime)),
		KeepaliveProbes:   gogoutils.UInt32GogoToProto(probes),
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

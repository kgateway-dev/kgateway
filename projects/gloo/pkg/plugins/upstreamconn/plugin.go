package upstreamconn

import (
	"math"

	"github.com/golang/protobuf/ptypes/duration"
	prototime "github.com/libopenstorage/openstorage/pkg/proto/time"
	"github.com/rotisserie/eris"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/wrappers"
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
		out.ConnectTimeout = cfg.ConnectTimeout
	}

	if cfg.TcpKeepalive != nil {
		out.UpstreamConnectionOptions = &envoyapi.UpstreamConnectionOptions{
			TcpKeepalive: convertTcpKeepAlive(cfg.TcpKeepalive),
		}
	}

	if cfg.PerConnectionBufferLimitBytes != nil {
		out.PerConnectionBufferLimitBytes = cfg.PerConnectionBufferLimitBytes
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

func convertTcpKeepAlive(tcp *v1.ConnectionConfig_TcpKeepAlive) *envoycore.TcpKeepalive {
	var probes *wrappers.UInt32Value
	if tcp.KeepaliveProbes > 0 {
		probes = &wrappers.UInt32Value{
			Value: tcp.KeepaliveProbes,
		}
	}
	return &envoycore.TcpKeepalive{
		KeepaliveInterval: roundToSecond(tcp.KeepaliveInterval),
		KeepaliveTime:     roundToSecond(tcp.KeepaliveTime),
		KeepaliveProbes:   probes,
	}
}

func convertHttpProtocolOptions(hpo *v1.ConnectionConfig_HttpProtocolOptions) (*envoycore.HttpProtocolOptions, error) {
	out := &envoycore.HttpProtocolOptions{}

	if hpo.IdleTimeout != nil {
		out.IdleTimeout = hpo.IdleTimeout
	}

	if hpo.MaxHeadersCount > 0 { // Envoy requires this to be >= 1
		out.MaxHeadersCount = &wrappers.UInt32Value{Value: hpo.MaxHeadersCount}
	}

	if hpo.MaxStreamDuration != nil {
		out.MaxStreamDuration = hpo.MaxStreamDuration
	}

	switch hpo.HeadersWithUnderscoresAction {
	case v1.ConnectionConfig_HttpProtocolOptions_ALLOW:
		out.HeadersWithUnderscoresAction = envoycore.HttpProtocolOptions_ALLOW
	case v1.ConnectionConfig_HttpProtocolOptions_REJECT_REQUEST:
		out.HeadersWithUnderscoresAction = envoycore.HttpProtocolOptions_REJECT_REQUEST
	case v1.ConnectionConfig_HttpProtocolOptions_DROP_HEADER:
		out.HeadersWithUnderscoresAction = envoycore.HttpProtocolOptions_DROP_HEADER
	default:
		return &envoycore.HttpProtocolOptions{},
			eris.Errorf("invalid HeadersWithUnderscoresAction %v in CommonHttpProtocolOptions", hpo.HeadersWithUnderscoresAction)
	}

	return out, nil
}

func roundToSecond(d *duration.Duration) *wrappers.UInt32Value {
	if d == nil {
		return nil
	}

	// round up
	seconds := math.Round(prototime.DurationFromProto(d).Seconds() + 0.4999)
	return &wrappers.UInt32Value{
		Value: uint32(seconds),
	}

}

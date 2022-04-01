package protocoloptions

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_upstreams_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	ExtensionName        = "protocol_options"
	MinWindowSize        = 65535
	MaxWindowSize        = 2147483647
	MinConcurrentStreams = 1
	MaxConcurrentStreams = 2147483647
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {

	if in.GetUseHttp2() == nil || !in.GetUseHttp2().GetValue() {
		return nil
	}
	// Both these values default to 268435456 if unset.
	sws := in.GetInitialStreamWindowSize()
	if sws != nil {
		if !validateWindowSize(sws.GetValue()) {
			return errors.Errorf("Invalid Initial Steam Window Size: %d", sws.GetValue())
		} else {
			sws = &wrappers.UInt32Value{Value: sws.GetValue()}
		}
	}

	cws := in.GetInitialConnectionWindowSize()
	if cws != nil {
		if !validateWindowSize(cws.GetValue()) {
			return errors.Errorf("Invalid Initial Connection Window Size: %d", cws.GetValue())
		} else {
			cws = &wrappers.UInt32Value{Value: cws.GetValue()}
		}
	}

	mcs := in.GetMaxConcurrentStreams()
	if mcs != nil {
		if !validateConcurrentStreams(mcs.GetValue()) {
			return errors.Errorf("Invalid Max Concurrent Streams Size: %d", mcs.GetValue())
		} else {
			mcs = &wrappers.UInt32Value{Value: mcs.GetValue()}
		}
	}

	protobuf := &envoy_extensions_upstreams_http_v3.HttpProtocolOptions{
		UpstreamProtocolOptions: &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{
						InitialStreamWindowSize:     sws,
						InitialConnectionWindowSize: cws,
						MaxConcurrentStreams:        mcs,
					},
				},
			},
		},
	}

	err := pluginutils.SetExtensionProtocolOptions(out, "envoy.extensions.upstreams.http.v3.HttpProtocolOptions", protobuf)
	if err != nil {
		return errors.Wrapf(err, "converting protocol options to struct")
	}

	return nil
}

func validateWindowSize(size uint32) bool {
	if size < MinWindowSize || size > MaxWindowSize {
		return false
	}
	return true
}

func validateConcurrentStreams(size uint32) bool {
	if size < MinConcurrentStreams || size > MaxConcurrentStreams {
		return false
	}
	return true
}

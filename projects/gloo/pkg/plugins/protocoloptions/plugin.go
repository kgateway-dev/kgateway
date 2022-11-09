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
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/httpprotocolhelpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/httpprotocolvalidation"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	ExtensionName = "protocol_options"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {

	if in.GetUseHttp2() == nil || !in.GetUseHttp2().GetValue() {
		return nil
	}

	//check if both http1 and http2 are being passed with protocol options set to USE_CONFIGURED
	//Both can be passed if protocol option is set to USE_DOWNSTREAM
	//Envoy enums - https://github.com/envoyproxy/envoy/blob/8259b33fea720672835d5c46722f0b97dfd69470/api/envoy/config/cluster/v3/cluster.proto#L152
	//Where the envoy error will be thrown - https://github.com/envoyproxy/envoy/blob/8259b33fea720672835d5c46722f0b97dfd69470/source/common/upstream/upstream_impl.cc#L771

	//At this point we know that we are passing the Http2 settings - check to make sure we aren't incorrectly passing http1 at the same time
	if in.GetProtocolSelection() == v1.Upstream_USE_CONFIGURED_PROTOCOL {
		if in.GetConnectionConfig().GetHttp1ProtocolOptions() != nil {
			return errors.Errorf(
				"Both HTTP1 and HTTP2 options may only be configured with non-default 'Upstream_USE_DOWNSTREAM_PROTOCOL' specified for Protocol Selection")
		}
	}

	// Both these values default to 268435456 if unset.
	sws := in.GetInitialStreamWindowSize()
	if sws != nil {
		if !httpprotocolvalidation.ValidateWindowSize(sws.GetValue()) {
			return errors.Errorf("Invalid Initial Stream Window Size: %d", sws.GetValue())
		} else {
			sws = &wrappers.UInt32Value{Value: sws.GetValue()}
		}
	}

	cws := in.GetInitialConnectionWindowSize()
	if cws != nil {
		if !httpprotocolvalidation.ValidateWindowSize(cws.GetValue()) {
			return errors.Errorf("Invalid Initial Connection Window Size: %d", cws.GetValue())
		} else {
			cws = &wrappers.UInt32Value{Value: cws.GetValue()}
		}
	}

	mcs := in.GetMaxConcurrentStreams()
	if mcs != nil {
		if !httpprotocolvalidation.ValidateConcurrentStreams(mcs.GetValue()) {
			return errors.Errorf("Invalid Max Concurrent Streams Size: %d", mcs.GetValue())
		} else {
			mcs = &wrappers.UInt32Value{Value: mcs.GetValue()}
		}
	}

	ose := in.GetOverrideStreamErrorOnInvalidHttpMessage()

	http2ProtocolOptions := &envoy_config_core_v3.Http2ProtocolOptions{
		InitialStreamWindowSize:                 sws,
		InitialConnectionWindowSize:             cws,
		MaxConcurrentStreams:                    mcs,
		OverrideStreamErrorOnInvalidHttpMessage: ose,
	}

	protobuf := &envoy_extensions_upstreams_http_v3.HttpProtocolOptions{}
	if in.GetProtocolSelection() == v1.Upstream_USE_DOWNSTREAM_PROTOCOL {
		var err error

		http1ProtocolOptions := &envoy_config_core_v3.Http1ProtocolOptions{}
		if in.GetConnectionConfig().GetHttp1ProtocolOptions() != nil {
			http1ProtocolOptions, err = httpprotocolhelpers.ConvertHttp1(*in.GetConnectionConfig().GetHttp1ProtocolOptions())
			if err != nil {
				return err
			}
		}

		protobuf.UpstreamProtocolOptions = &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_UseDownstreamProtocolConfig{
			UseDownstreamProtocolConfig: &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_UseDownstreamHttpConfig{
				HttpProtocolOptions:  http1ProtocolOptions,
				Http2ProtocolOptions: http2ProtocolOptions,
			},
		}
	} else {
		protobuf.UpstreamProtocolOptions = &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_extensions_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: http2ProtocolOptions,
				},
			},
		}
	}

	err := pluginutils.SetExtensionProtocolOptions(out, "envoy.extensions.upstreams.http.v3.HttpProtocolOptions", protobuf)
	if err != nil {
		return errors.Wrapf(err, "converting protocol options to struct")
	}

	return nil
}

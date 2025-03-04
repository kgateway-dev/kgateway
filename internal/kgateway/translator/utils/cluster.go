package utils

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	proto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func MutateHttpOptions(c *envoy_config_cluster_v3.Cluster, m func(*envoy_upstreams_v3.HttpProtocolOptions)) error {
	if c.GetTypedExtensionProtocolOptions() == nil {
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{}
	}
	http2ProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{}
	if opts, ok := c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]; ok {
		err := anypb.UnmarshalTo(opts, http2ProtocolOptions, proto.UnmarshalOptions{})
		if err != nil {
			return err
		}
	}
	m(http2ProtocolOptions)

	a, err := utils.MessageToAny(http2ProtocolOptions)
	if err != nil {
		return err
	}

	c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = a
	return nil
}

func SetHttp2options(c *envoy_config_cluster_v3.Cluster) error {
	return MutateHttpOptions(c, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{},
				},
			},
		}
	})
}

package headers

import (
	"strings"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.RoutePlugin               = new(plugin)
	_ plugins.VirtualHostPlugin         = new(plugin)
	_ plugins.WeightedDestinationPlugin = new(plugin)
)

const (
	ExtensionName = "headers"
)

var (
	MissingHeaderValueError = eris.Errorf("header section of header value option cannot be nil")
	CantSetHostHeaderError  = eris.Errorf("cannot set Host header in response headers")

	CantSetPseudoHeaderError = func(header string) error {
		return eris.Errorf(":-prefixed headers cannot be set: %s", header)
	}
)

// Puts Header Manipulation config on Routes, VirtualHosts, and Weighted Clusters
type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()
	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot))
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot))
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot))
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

type envoyHeaderManipulation struct {
	RequestHeadersToAdd     []*envoy_config_core_v3.HeaderValueOption
	RequestHeadersToRemove  []string
	ResponseHeadersToAdd    []*envoy_config_core_v3.HeaderValueOption
	ResponseHeadersToRemove []string
}

func getSecretsFromSnapshot(snapshot *v1snap.ApiSnapshot) *v1.SecretList {
	var secrets *v1.SecretList
	if snapshot == nil {
		secrets = &v1.SecretList{}
	} else {
		secrets = &snapshot.Secrets
	}
	return secrets
}

func convertHeaderConfig(in *headers.HeaderManipulation, secrets *v1.SecretList) (*envoyHeaderManipulation, error) {
	// request headers can either be made from a normal key/value pair, or.
	// they can be constructed from a supplied secret. To accomplish this, we use
	// a utility function that was originally created to accomplish this for health check headers.
	requestAdd, err := api_conversion.ToEnvoyHeaderValueOptionList(in.GetRequestHeadersToAdd(), secrets)
	if err != nil {
		return nil, err
	}
	// response headers have no reason to include secrets.
	responseAdd, err := convertResponseHeaderValueOption(in.GetResponseHeadersToAdd())
	if err != nil {
		return nil, err
	}

	return &envoyHeaderManipulation{
		RequestHeadersToAdd:     requestAdd,
		RequestHeadersToRemove:  in.GetRequestHeadersToRemove(),
		ResponseHeadersToAdd:    responseAdd,
		ResponseHeadersToRemove: in.GetResponseHeadersToRemove(),
	}, nil
}

func convertResponseHeaderValueOption(
	in []*headers.HeaderValueOption,
) ([]*envoy_config_core_v3.HeaderValueOption, error) {
	var out []*envoy_config_core_v3.HeaderValueOption
	for _, h := range in {
		header := h.GetHeader()
		if header == nil {
			return nil, MissingHeaderValueError
		}

		if header.GetKey()[0] == byte(':') {
			return nil, CantSetPseudoHeaderError(header.GetKey())
		}

		if strings.EqualFold(header.GetKey(), "Host") {
			return nil, CantSetHostHeaderError
		}

		out = append(out, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   header.GetKey(),
				Value: header.GetValue(),
			},
			Append: h.GetAppend(),
		})
	}
	return out, nil
}

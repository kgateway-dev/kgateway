package headers

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/headers"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	MissingHeaderValueError = errors.Errorf("header section of header value option cannot be nil")
)

// Puts Header Manipulation config on Routes, VirtualHosts, and Weighted Clusters
type Plugin struct{}

var _ plugins.RoutePlugin = NewPlugin()
var _ plugins.VirtualHostPlugin = NewPlugin()
var _ plugins.WeightedDestinationPlugin = NewPlugin()

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessWeightedDestination(_ plugins.RouteParams, in *v1.WeightedDestination, out *envoyroute.WeightedCluster_ClusterWeight) error {
	headerManipulation := in.GetWeightedDestinationPlugins().GetHeaderManipulation()
	if headerManipulation == nil {
		// Try deprecated field
		headerManipulation = in.GetWeighedDestinationPlugins().GetHeaderManipulation()
		if headerManipulation == nil {
			return nil
		}
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation)
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	headerManipulation := in.GetVirtualHostPlugins().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation)
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	headerManipulation := in.GetRoutePlugins().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	envoyHeader, err := convertHeaderConfig(headerManipulation)
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
	RequestHeadersToAdd     []*envoycore.HeaderValueOption
	RequestHeadersToRemove  []string
	ResponseHeadersToAdd    []*envoycore.HeaderValueOption
	ResponseHeadersToRemove []string
}

func convertHeaderConfig(in *headers.HeaderManipulation) (*envoyHeaderManipulation, error) {
	requestAdd, err := convertHeaderValueOption(in.GetRequestHeadersToAdd())
	if err != nil {
		return nil, err
	}
	responseAdd, err := convertHeaderValueOption(in.GetResponseHeadersToAdd())
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

func convertHeaderValueOption(in []*headers.HeaderValueOption) ([]*envoycore.HeaderValueOption, error) {
	var out []*envoycore.HeaderValueOption
	for _, h := range in {
		if h.Header == nil {
			return nil, MissingHeaderValueError
		}
		out = append(out, &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   h.Header.Key,
				Value: h.Header.Value,
			},
			Append: h.Append,
		})
	}
	return out, nil
}

package headermodifier

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx *filterplugins.RouteContext,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	if filter.Type == gwv1.HTTPRouteFilterRequestHeaderModifier {
		return p.applyRequestFilter(filter.RequestHeaderModifier, outputRoute)
	}
	if filter.Type == gwv1.HTTPRouteFilterResponseHeaderModifier {
		return p.applyResponseFilter(filter.ResponseHeaderModifier, outputRoute)
	}
	return errors.Errorf("unsupported filter type: %v", filter.Type)
}

func (p *Plugin) applyRequestFilter(
	config *gwv1.HTTPHeaderFilter,
	outputRoute *v1.Route,
) error {
	if config == nil {
		return errors.Errorf("RequestHeaderModifier filter supplied does not define requestHeaderModifier")
	}
	headerManipulation := outputRoute.Options.HeaderManipulation
	if headerManipulation == nil {
		headerManipulation = &headers.HeaderManipulation{}
	}
	headerManipulation.RequestHeadersToAdd = requestHeadersToAdd(config.Add, config.Set)
	headerManipulation.RequestHeadersToRemove = config.Remove
	outputRoute.Options.HeaderManipulation = headerManipulation
	return nil
}

func (p *Plugin) applyResponseFilter(
	config *gwv1.HTTPHeaderFilter,
	outputRoute *v1.Route,
) error {
	if config == nil {
		return errors.Errorf("Response filter supplied does not define requestHeaderModifier")
	}
	headerManipulation := outputRoute.Options.HeaderManipulation
	if headerManipulation == nil {
		headerManipulation = &headers.HeaderManipulation{}
	}
	headerManipulation.ResponseHeadersToAdd = responseHeadersToAdd(config.Add, config.Set)
	headerManipulation.ResponseHeadersToRemove = config.Remove
	outputRoute.Options.HeaderManipulation = headerManipulation
	return nil
}

func requestHeadersToAdd(add []gwv1.HTTPHeader, set []gwv1.HTTPHeader) []*core.HeaderValueOption {
	envoyHeaders := make([]*core.HeaderValueOption, 0, len(add)+len(set))
	envoyHeaders = append(envoyHeaders, translateHeaders(add, true)...)
	envoyHeaders = append(envoyHeaders, translateHeaders(set, false)...)
	return envoyHeaders
}

func translateHeaders(gwHeaders []gwv1.HTTPHeader, add bool) []*core.HeaderValueOption {
	var envoyHeaders []*core.HeaderValueOption
	for _, gwHeader := range gwHeaders {
		envoyHeaders = append(envoyHeaders, &core.HeaderValueOption{
			HeaderOption: &core.HeaderValueOption_Header{
				Header: &core.HeaderValue{
					Key:   string(gwHeader.Name),
					Value: gwHeader.Value,
				},
			},
			Append: &wrappers.BoolValue{Value: add},
		})
	}
	return envoyHeaders
}

func responseHeadersToAdd(add []gwv1.HTTPHeader, set []gwv1.HTTPHeader) []*headers.HeaderValueOption {
	envoyHeaders := make([]*headers.HeaderValueOption, 0, len(add)+len(set))
	envoyHeaders = append(envoyHeaders, translateResponseHeaders(add, true)...)
	envoyHeaders = append(envoyHeaders, translateResponseHeaders(set, false)...)
	return envoyHeaders
}

func translateResponseHeaders(gwHeaders []gwv1.HTTPHeader, add bool) []*headers.HeaderValueOption {
	var envoyHeaders []*headers.HeaderValueOption
	for _, gwHeader := range gwHeaders {
		envoyHeaders = append(envoyHeaders, &headers.HeaderValueOption{
			Header: &headers.HeaderValue{
				Key:   string(gwHeader.Name),
				Value: gwHeader.Value,
			},
			Append: &wrappers.BoolValue{Value: add},
		})
	}
	return envoyHeaders
}

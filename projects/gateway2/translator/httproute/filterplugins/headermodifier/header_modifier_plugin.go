package headermodifier

import (
	"context"

	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
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
	ctx context.Context,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	if filter.Type == gwv1.HTTPRouteFilterRequestHeaderModifier {
		return p.applyRequestFilter(filter.RequestHeaderModifier, outputRoute)
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

func requestHeadersToAdd(add []gwv1.HTTPHeader, set []gwv1.HTTPHeader) []*core.HeaderValueOption {
	var envoyHeaders []*core.HeaderValueOption
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

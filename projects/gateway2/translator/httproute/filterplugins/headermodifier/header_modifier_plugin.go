package headermodifier

import (
	"context"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	errors "github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx context.Context,
	filter gwv1.HTTPRouteFilter,
	outputRoute *routev3.Route,
) error {
	if filter.Type == gwv1.HTTPRouteFilterRequestHeaderModifier {
		return p.applyRequestFilter(filter.RequestHeaderModifier, outputRoute)
	}
	return errors.Errorf("unsupported filter type: %v", filter.Type)
}

func (p *Plugin) applyRequestFilter(
	config *gwv1.HTTPHeaderFilter,
	outputRoute *routev3.Route,
) error {
	if config == nil {
		return errors.Errorf("RequestHeaderModifier filter supplied does not define requestHeaderModifier")
	}
	outputRoute.RequestHeadersToAdd = requestHeadersToAdd(config.Add, config.Set)
	outputRoute.RequestHeadersToRemove = config.Remove
	return nil
}

func requestHeadersToAdd(add []gwv1.HTTPHeader, set []gwv1.HTTPHeader) []*corev3.HeaderValueOption {
	var envoyHeaders []*corev3.HeaderValueOption
	envoyHeaders = append(envoyHeaders, translateHeaders(add, true)...)
	envoyHeaders = append(envoyHeaders, translateHeaders(set, false)...)
	return envoyHeaders
}

func translateHeaders(gwHeaders []gwv1.HTTPHeader, add bool) []*corev3.HeaderValueOption {
	var envoyHeaders []*corev3.HeaderValueOption
	for _, gwHeader := range gwHeaders {
		envoyHeaders = append(envoyHeaders, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   string(gwHeader.Name),
				Value: gwHeader.Value,
			},
			Append: wrapperspb.Bool(add),
		})
	}
	return envoyHeaders
}

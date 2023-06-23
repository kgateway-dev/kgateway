package helpers

import v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

// EndpointBuilder contains options for building Endpoints to be included in scaled Snapshots
// there are no options currently configurable for the endpointBuilder
type EndpointBuilder struct{}

func NewEndpointBuilder() *EndpointBuilder {
	return &EndpointBuilder{}
}

func (b *EndpointBuilder) Build(i int) *v1.Endpoint {
	return Endpoint(i)
}

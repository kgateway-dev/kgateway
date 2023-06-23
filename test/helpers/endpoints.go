package helpers

import v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

// endpointBuilder contains options for building Endpoints to be included in scaled Snapshots
// there are no options currently configurable for the endpointBuilder
type endpointBuilder struct{}

func NewEndpointBuilder() *endpointBuilder {
	return &endpointBuilder{}
}

func (b *endpointBuilder) Build(i int) *v1.Endpoint {
	return Endpoint(i)
}

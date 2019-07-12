package v1

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
)

/*
	These interfaces should be implemented by upstreams that support subset load balancing.
	https://github.com/envoyproxy/envoy/blob/master/source/docs/subset_load_balancer.md
*/

type SubsetSpecGetter interface {
	GetSubsetSpec() *plugins.SubsetSpec
}
type SubsetSpecSetter interface {
	SetSubsetSpec(*plugins.SubsetSpec)
}
type SubsetSpecMutator interface {
	SubsetSpecGetter
	SubsetSpecSetter
}

func (us *UpstreamSpec_Kube) GetSubsetSpec() *plugins.SubsetSpec {
	return us.Kube.SubsetSpec
}

func (us *UpstreamSpec_Kube) SetSubsetSpec(spec *plugins.SubsetSpec) {
	us.Kube.SubsetSpec = spec
}

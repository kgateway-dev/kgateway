package v1

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
)

type ServiceSpecGetter interface {
	GetServiceSpec() *plugins.ServiceSpec
}
type ServiceSpecSetter interface {
	SetServiceSpec(*plugins.ServiceSpec)
}
type ServiceSpecMutator interface {
	ServiceSpecGetter
	ServiceSpecSetter
}

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

/*
	Add these two methods to any upstream spec that supports
*/
func (us *UpstreamSpec_Kube) GetServiceSpec() *plugins.ServiceSpec {
	return us.Kube.ServiceSpec
}

func (us *UpstreamSpec_Kube) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Kube.ServiceSpec = spec
}

func (us *UpstreamSpec_Static) GetServiceSpec() *plugins.ServiceSpec {
	return us.Static.ServiceSpec
}

func (us *UpstreamSpec_Static) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Static.ServiceSpec = spec
}

func (us *UpstreamSpec_Consul) GetServiceSpec() *plugins.ServiceSpec {
	return us.Consul.ServiceSpec
}

func (us *UpstreamSpec_Consul) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Consul.ServiceSpec = spec
}

func (us *UpstreamSpec_Kube) GetSubsetSpec() *plugins.SubsetSpec {
	return us.Kube.SubsetSpec
}

func (us *UpstreamSpec_Kube) SetSubsetSpec(spec *plugins.SubsetSpec) {
	us.Kube.SubsetSpec = spec
}

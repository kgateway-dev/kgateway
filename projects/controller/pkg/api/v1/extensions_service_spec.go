package v1

import (
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
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

/*
Add these two methods to any upstream spec that supports a ServiceSpec
describing the service represented by the upstream
*/
func (us *Upstream_Kube) GetServiceSpec() *plugins.ServiceSpec {
	return us.Kube.GetServiceSpec()
}

func (us *Upstream_Kube) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Kube.ServiceSpec = spec
}

func (us *Upstream_Static) GetServiceSpec() *plugins.ServiceSpec {
	return us.Static.GetServiceSpec()
}

func (us *Upstream_Static) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Static.ServiceSpec = spec
}

func (us *Upstream_Pipe) GetServiceSpec() *plugins.ServiceSpec {
	return us.Pipe.GetServiceSpec()
}

func (us *Upstream_Pipe) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Pipe.ServiceSpec = spec
}

func (us *Upstream_Consul) GetServiceSpec() *plugins.ServiceSpec {
	return us.Consul.GetServiceSpec()
}

func (us *Upstream_Consul) SetServiceSpec(spec *plugins.ServiceSpec) {
	us.Consul.ServiceSpec = spec
}

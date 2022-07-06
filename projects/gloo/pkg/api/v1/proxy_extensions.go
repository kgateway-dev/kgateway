package v1

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

var _ resources.CustomInputResource = &Proxy{}

func (p *Proxy) UnmarshalSpec(spec v1.Spec) error {
	return compress.UnmarshalSpec(p, spec)
}

func (p *Proxy) MarshalSpec() (v1.Spec, error) {
	return compress.MarshalSpec(p)
}

func (p *Proxy) UnmarshalStatus(status v1.Status, unmarshaler resources.StatusUnmarshaler) {
	compress.UnmarshalStatus(p, status, unmarshaler)
}

func (p *Proxy) MarshalStatus() (v1.Status, error) {
	return compress.MarshalStatus(p)
}

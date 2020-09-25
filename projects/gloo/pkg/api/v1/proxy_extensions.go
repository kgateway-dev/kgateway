package v1

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

var _ resources.CustomInputResource = &Proxy{}

const compressedSpec = "compressedSpec"

func (p *Proxy) UnmarshalSpec(spec v1.Spec) error {

	// do we have compressed spec?
	// compress it
	// if we don't, don't

	if _, ok := spec[compressedSpec]; ok {
		spec = uncompressSpec(spec)
		p.setCompressed()
	}
	if err := protoutils.UnmarshalMap(spec, p); err != nil {
		return errors.Wrapf(err, "reading crd spec on resource %v in namespace %v into Proxy", p.GetMetadata().Name, p.GetMetadata().namespace)
	}
	return nil
}

func (p *Proxy) MarshalSpec() (v1.Spec, error) {

	data, err := protoutils.MarshalMap(p)
	if err != nil {
		return nil, crd.MarshalErr(err, "resource to map")
	}

	delete(data, "metadata")
	delete(data, "status")
	// save this as usual:
	var spec v1.Spec
	spec = data
	if p.shouldCompress() {
		spec = compressSpec(spec)
	}
	return spec, nil
}

func (p *Proxy) UnmarshalStatus(status v1.Status) error {
	typedStatus := core.Status{}
	if err := protoutils.UnmarshalMapToProto(status, &typedStatus); err != nil {
		return err
	}
	p.Status = typedStatus
	return nil
}

func (p *Proxy) MarshalStatus() (v1.Status, error) {
	statusProto := p.GetStatus()
	statusMap, err := protoutils.MarshalMapFromProtoWithEnumsAsInts(&statusProto)
	if err != nil {
		return nil, crd.MarshalErr(err, "resource status to map")
	}
	return statusMap, nil
}

func (p *Proxy) shouldCompress() bool {
	return true
}

func (p *Proxy) setCompressed() {
}

func compressSpec(s v1.Spec) v1.Spec {
	newspec := v1.Spec{}

	// serialize spec to json:
	//ser,err:=jsonpb.

	// base64 encode

	// write it
	return s
}

func uncompressSpec(s v1.Spec) v1.Spec {
	// TODO: really uncompress
	return s
}

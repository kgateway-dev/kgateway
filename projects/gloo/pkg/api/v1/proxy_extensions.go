package v1

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	v1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

var _ resources.CustomInputResource = &Proxy{}

func (p *Proxy) UnmarshalSpec(spec v1.Spec) error {
	if compress.IsCompressed(spec) {
		var err error
		spec, err = compress.UncompressSpec(spec)
		if err != nil {
			return errors.Wrapf(err, "reading unmarshalling spec on resource %v in namespace %v into Proxy", p.GetMetadata().Name, p.GetMetadata().Namespace)
		}
		// if we have a compressed spec, make sure the proxy is marked for compression
		compress.SetShouldCompressed(p)
	}
	if err := protoutils.UnmarshalMap(spec, p); err != nil {
		return errors.Wrapf(err, "reading crd spec on resource %v in namespace %v into Proxy", p.GetMetadata().Name, p.GetMetadata().Namespace)
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
	if compress.ShouldCompress(p) {
		spec, err = compress.CompressSpec(spec)
		if err != nil {
			return nil, errors.Wrapf(err, "reading marshalling spec on resource %v in namespace %v into Proxy", p.GetMetadata().Name, p.GetMetadata().Namespace)
		}
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

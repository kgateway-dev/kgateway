package v1

import (
	bytes "bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

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
	if _, ok := spec[compressedSpec]; ok {
		var err error
		spec, err = uncompressSpec(spec)
		if err != nil {
			return errors.Wrapf(err, "reading unmarshalling spec on resource %v in namespace %v into Proxy", p.GetMetadata().Name, p.GetMetadata().Namespace)
		}
		p.setCompressed()
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
	if p.shouldCompress() {
		spec, err = compressSpec(spec)
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

func (p *Proxy) shouldCompress() bool {
	return true
}

func (p *Proxy) setCompressed() {
}

func compressSpec(s v1.Spec) (v1.Spec, error) {
	// serialize  spec to json:
	ser, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(ser)
	w.Close()

	newSpec := v1.Spec{}
	newSpec[compressedSpec] = b.Bytes()
	return newSpec, nil
}

func uncompressSpec(s v1.Spec) (v1.Spec, error) {

	compressed, ok := s[compressedSpec]
	if !ok {
		return nil, fmt.Errorf("not compressed")
	}

	var spec v1.Spec
	switch data := compressed.(type) {
	case []byte:
		err := json.Unmarshal(data, &spec)
		if err != nil {
			return nil, crd.MarshalErr(err, "data not json")
		}

		return spec, nil
	case string:
		decodedData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, crd.MarshalErr(err, "data not base64")
		}

		var b bytes.Buffer
		r, err := zlib.NewReader(bytes.NewBuffer(decodedData))
		io.Copy(&b, r)
		r.Close()

		err = json.Unmarshal(b.Bytes(), &spec)
		if err != nil {
			return nil, crd.MarshalErr(err, "data is not valid json")
		}
		return spec, nil
	default:
		return nil, fmt.Errorf("unknown datatype %T", compressed)
	}
}

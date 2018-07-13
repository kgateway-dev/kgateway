package kubernetes

import (
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
)

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (*UpstreamSpec, error) {
	s := new(UpstreamSpec)
	if err := protoutil.UnmarshalStruct(generic, s); err != nil {
		return nil, err
	}
	return s, s.validateUpstream()
}

func EncodeUpstreamSpec(spec UpstreamSpec) *types.Struct {
	pb, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return pb
}

func (s *UpstreamSpec) validateUpstream() error {
	if s.ServiceName == "" {
		return errors.New("service name must be set")
	}
	if s.ServiceNamespace == "" {
		return errors.New("service namespace must be set")
	}
	return nil
}

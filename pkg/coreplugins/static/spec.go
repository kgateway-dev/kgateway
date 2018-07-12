package static

import (
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (*UpstreamSpec, error) {
	var s UpstreamSpec
	if err := util.StructToMessage(generic, &s); err != nil {
		return &s, err
	}
	return &s, s.validateUpstream()
}

func EncodeUpstreamSpec(spec *UpstreamSpec) v1.UpstreamSpec {
	v1Spec, err := util.MessageToStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

func (s *UpstreamSpec) validateUpstream() error {
	if len(s.Hosts) == 0 {
		return errors.New("most provide at least 1 host")
	}
	for _, host := range s.Hosts {
		if host.Addr == "" {
			return errors.New("ip cannot be empty")
		}
		if host.Port == 0 {
			return errors.New("port cannot be empty")
		}
	}
	return nil
}

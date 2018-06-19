package static

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
)

type UpstreamSpec struct {
	Hosts      []Host `json:"hosts"`
	EnableIPv6 bool   `json:"enable_ipv6"`
	TLS        *bool   `json:"tls"`
}

type Host struct {
	Addr string `json:"addr"`
	Port uint32 `json:"port"`
}

func DecodeUpstreamSpec(generic v1.UpstreamSpec) (UpstreamSpec, error) {
	var s UpstreamSpec
	if err := protoutil.UnmarshalStruct(generic, &s); err != nil {
		return s, err
	}
	return s, s.validateUpstream()
}

func EncodeUpstreamSpec(spec UpstreamSpec) v1.UpstreamSpec {
	v1Spec, err := protoutil.MarshalStruct(spec)
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

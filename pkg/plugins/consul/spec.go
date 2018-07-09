package consul

import (
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
)

type UpstreamSpec struct {
	ServiceName string   `json:"service_name"`
	ServiceTags []string `json:"service_tags"`
	Connect     *Connect `json:"connect"`
}

// Indicates whether this upstream is part of the Consul Connect mesh
type Connect struct {
	TlsSecretRef string `json:"tls_secret_ref"`
}

const (
	LeafCertificateSecret = "certificates"
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
	return nil
}

// TODO(ilackarms + yuval-k): assess where this function should ive
func UpstreamNameForConnectService(serviceName string) string {
	return serviceName+"-proxy"
}
package grpc

import (
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/protoutil"
)

func DecodeServiceProperties(generic *types.Struct) (ServiceProperties, error) {
	var p ServiceProperties
	err := protoutil.UnmarshalStruct(generic, &p)
	return p, err
}

func EncodeServiceProperties(properties ServiceProperties) *types.Struct {
	v1Properties, err := protoutil.MarshalStruct(properties)
	if err != nil {
		panic(err)
	}
	return v1Properties
}

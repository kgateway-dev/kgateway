package proto

import (
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

func GetMessage(protos map[string]*types.Any, name string) proto.Message {
	if any, ok := protos[name]; ok {
		return getProto(any)
	}

	return nil
}

func getProto(p *types.Any) proto.Message {
	var x types.DynamicAny
	types.UnmarshalAny(p, &x)
	return x.Message
}

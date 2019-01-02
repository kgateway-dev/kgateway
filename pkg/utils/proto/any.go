package proto

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

func GetMessage(protos map[string]*types.Any, name string) (proto.Message, error) {
	if any, ok := protos[name]; ok {
		return getProto(any)
	}

	return nil, fmt.Errorf("message not found")
}

func getProto(p *types.Any) (proto.Message, error) {
	var x types.DynamicAny
	err := types.UnmarshalAny(p, &x)
	if err != nil {
		return nil, err
	}
	return x.Message, nil
}

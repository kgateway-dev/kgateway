package proto

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

var NotFoundError = fmt.Errorf("message not found")

func GetMessage(protos map[string]*types.Any, name string) (proto.Message, error) {
	if any, ok := protos[name]; ok {
		return getProto(any)
	}

	return nil, NotFoundError
}

func getProto(p *types.Any) (proto.Message, error) {
	var x types.DynamicAny
	err := types.UnmarshalAny(p, &x)
	if err != nil {
		return nil, err
	}
	return x.Message, nil
}

package proto

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

var NotFoundError = fmt.Errorf("message not found")

func UnmarshalAnyFromMap(protos map[string]*types.Any, name string, outproto proto.Message) error {
	if any, ok := protos[name]; ok {
		return getProto(any, outproto)
	}
	return NotFoundError
}

func getProto(p *types.Any, outproto proto.Message) error {
	err := types.UnmarshalAny(p, outproto)
	if err != nil {
		return err
	}
	return nil
}

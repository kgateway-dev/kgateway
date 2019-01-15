package utils

import (
	"fmt"
	"reflect"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/envoyproxy/go-control-plane/pkg/util"
)

var NotFoundError = fmt.Errorf("message not found")

type PluginContainer interface {
	GetPlugins() map[string]*types.Any
}

func UnmarshalAnyPlugins(plugins PluginContainer, name string, outproto proto.Message) error {
	if plugins == nil {
		return NotFoundError
	}

	// value might still be a typed nil, so test for that too.
	if reflect.ValueOf(plugins).IsNil() {
		return NotFoundError
	}

	pluginmap := plugins.GetPlugins()
	if pluginmap == nil {
		return NotFoundError
	}

	return UnmarshalAnyFromMap(pluginmap, name, outproto)
}

func UnmarshalAnyFromMap(protos map[string]*types.Any, name string, outproto proto.Message) error {
	if any, ok := protos[name]; ok {
		return getProto(any, outproto)
	}
	return NotFoundError
}

func getProto(p *types.Any, outproto proto.Message) error {

	// special case - if we have a struct, use json pb for it.
	if p.TypeUrl == "type.googleapis.com/google.protobuf.Struct" {
		var msg types.Struct
		err := types.UnmarshalAny(p, &msg)
		if err != nil {
			return err
		}
		return util.StructToMessage(&msg, outproto)
	}

	err := types.UnmarshalAny(p, outproto)
	if err != nil {
		return err
	}
	return nil
}

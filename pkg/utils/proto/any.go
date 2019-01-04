package proto

import (
	"fmt"
	"reflect"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
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
	err := types.UnmarshalAny(p, outproto)
	if err != nil {
		return err
	}
	return nil
}

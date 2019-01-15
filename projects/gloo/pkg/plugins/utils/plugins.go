package utils

import (
	"fmt"
	"reflect"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/envoyproxy/go-control-plane/pkg/util"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var NotFoundError = fmt.Errorf("message not found")

type extensionPluginContainer interface {
	GetExtensions() *v1.Extensions
}

func UnmarshalExtension(extensions extensionPluginContainer, name string, outproto proto.Message) error {
	if extensions == nil {
		return NotFoundError
	}

	// value might still be a typed nil, so test for that too.
	if reflect.ValueOf(extensions).IsNil() {
		return NotFoundError
	}

	extensionMap := extensions.GetExtensions()
	if extensionMap == nil {
		return NotFoundError
	}

	return UnmarshalStructFromMap(extensionMap.GetConfigs(), name, outproto)
}

func UnmarshalStructFromMap(protos map[string]*types.Struct, name string, outproto proto.Message) error {
	if msg, ok := protos[name]; ok {
		return util.StructToMessage(msg, outproto)
	}
	return NotFoundError
}

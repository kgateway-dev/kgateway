package pluginutils

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
)

func SetExtenstionProtocolOptions(out *envoyapi.Cluster, filterName string, protoext proto.Message) error {

	if out.ExtensionProtocolOptions == nil {
		out.TypedExtensionProtocolOptions = make(map[string]*types.Any)
	}

	marshalledProtoExt, err := types.MarshalAny(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	out.TypedExtensionProtocolOptions[filterName] = marshalledProtoExt
	return nil
}

func GetExtenstionProtocolOptions(out *envoyapi.Cluster, filterName string, protoext proto.Message) error {
	if out.TypedExtensionProtocolOptions == nil {
		return nil
	}
	if marshalledProtoExt, ok := out.TypedExtensionProtocolOptions[filterName]; ok {
		return types.UnmarshalAny(marshalledProtoExt, protoext)
	}
	return nil
}

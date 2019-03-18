package pluginutils

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
)

func SetExtenstionProtocolOptions(out *envoyapi.Cluster, filterName string, protoext proto.Message) error {

	if out.ExtensionProtocolOptions == nil {
		out.ExtensionProtocolOptions = make(map[string]*types.Struct)
	}

	protoextStruct, err := util.MessageToStruct(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	out.ExtensionProtocolOptions[filterName] = protoextStruct
	return nil
}

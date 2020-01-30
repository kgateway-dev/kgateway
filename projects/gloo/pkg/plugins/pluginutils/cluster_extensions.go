package pluginutils

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/gogo/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	errors "github.com/rotisserie/eris"
)

func SetExtenstionProtocolOptions(out *envoyapi.Cluster, filterName string, protoext proto.Message) error {

	if out.ExtensionProtocolOptions == nil {
		out.ExtensionProtocolOptions = make(map[string]*structpb.Struct)
	}

	protoextStruct, err := conversion.MessageToStruct(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	out.ExtensionProtocolOptions[filterName] = protoextStruct
	return nil
}

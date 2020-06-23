package pluginutils

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	anypb "github.com/golang/protobuf/ptypes/any"
	errors "github.com/rotisserie/eris"
)

func SetExtenstionProtocolOptions(out *envoyapi.Cluster, filterName string, protoext proto.Message) error {
	protoextAny, err := MessageToAny(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	if out.TypedExtensionProtocolOptions == nil {
		out.TypedExtensionProtocolOptions = make(map[string]*anypb.Any)
	}

	out.TypedExtensionProtocolOptions[filterName] = protoextAny
	return nil

}

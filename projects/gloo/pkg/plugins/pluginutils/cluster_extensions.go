package pluginutils

import (
	"fmt"

	udpa "github.com/cncf/udpa/go/udpa/type/v1"
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/proto"
	goproto "github.com/golang/protobuf/proto"
	anypb "github.com/golang/protobuf/ptypes/any"
	errors "github.com/rotisserie/eris"
)

func SetExtenstionProtocolOptions(out *envoyapi.Cluster, filterName string, protoext proto.Message) error {

	// convert to sturct first. we cant convert to any as we use gogo proto, and control plane
	// uses go proto.
	protoextStruct, err := conversion.MessageToStruct(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	typedStruct := &udpa.TypedStruct{
		TypeUrl: "type.googleapis.com/",
		Value:   protoextStruct,
	}

	if s := gogoproto.MessageName(protoext); s != "" {
		typedStruct.TypeUrl += s
	} else if s := goproto.MessageName(protoext); s != "" {
		typedStruct.TypeUrl += s
	} else {
		return fmt.Errorf("can't determine message name")
	}

	if out.TypedExtensionProtocolOptions == nil {
		out.TypedExtensionProtocolOptions = make(map[string]*anypb.Any)
	}

	protoextAny, err := MessageToAny(typedStruct)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	out.TypedExtensionProtocolOptions[filterName] = protoextAny
	return nil

}

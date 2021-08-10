package utils

import (
	"fmt"

	udpa_type_v1 "github.com/cncf/udpa/go/udpa/type/v1"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/golang/protobuf/proto"
	gogoproto "github.com/golang/protobuf/proto"
	goproto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	pany "github.com/golang/protobuf/ptypes/any"
)

func MessageToAny(msg proto.Message) (*pany.Any, error) {

	name, err := protoToMessageName(msg)
	if err != nil {
		return nil, err
	}
	buf, err := protoToMessageBytes(msg)
	if err != nil {
		return nil, err
	}
	return &pany.Any{
		TypeUrl: name,
		Value:   buf,
	}, nil
}

func MustMessageToAny(msg proto.Message) *pany.Any {
	anymsg, err := MessageToAny(msg)
	if err != nil {
		panic(err)
	}
	return anymsg
}

func AnyToMessage(a *pany.Any) (proto.Message, error) {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	return x.Message, err
}

func MustAnyToMessage(a *pany.Any) proto.Message {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	if err != nil {
		panic(err)
	}
	return x.Message
}

// gogoprotos converted directly to goproto any can't be marshalled unless you wrap
// the contents of the gogoproto in a typed struct
func MustGogoMessageToAnyGoProto(msg proto.Message) *pany.Any {
	any, err := GogoMessageToAnyGoProto(msg)
	if err != nil {
		panic(err)
	}
	return any
}

// gogoprotos converted directly to goproto any can't be marshalled unless you wrap
// the contents of the gogoproto in a typed struct
func GogoMessageToAnyGoProto(msg proto.Message) (*pany.Any, error) {
	configStruct, err := conversion.MessageToStruct(msg)
	if err != nil {
		return nil, err
	}

	anyGogo := MustMessageToAny(msg)

	// create a typed struct so go proto can handle marshalling any types derived from gogo protos
	ts := &udpa_type_v1.TypedStruct{Value: configStruct, TypeUrl: anyGogo.GetTypeUrl()}
	tsAnyGo := MustMessageToAny(ts)

	anyGo := &pany.Any{Value: tsAnyGo.GetValue(), TypeUrl: tsAnyGo.GetTypeUrl()}
	return anyGo, nil
}

func protoToMessageName(msg proto.Message) (string, error) {
	typeUrlPrefix := "type.googleapis.com/"

	if s := gogoproto.MessageName(msg); s != "" {
		return typeUrlPrefix + s, nil
	} else if s := goproto.MessageName(msg); s != "" {
		return typeUrlPrefix + s, nil
	}
	return "", fmt.Errorf("can't determine message name")
}

func protoToMessageBytes(msg proto.Message) ([]byte, error) {
	if b, err := protoToMessageBytesGolang(msg); err == nil {
		return b, nil
	}
	return protoToMessageBytesGogo(msg)
}

func protoToMessageBytesGogo(msg proto.Message) ([]byte, error) {
	b := gogoproto.NewBuffer(nil)
	b.SetDeterministic(true)
	err := b.Marshal(msg)
	return b.Bytes(), err
}

func protoToMessageBytesGolang(msg proto.Message) ([]byte, error) {
	b := proto.NewBuffer(nil)
	b.SetDeterministic(true)
	err := b.Marshal(msg)
	return b.Bytes(), err
}

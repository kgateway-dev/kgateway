package pluginutils

import (
	"fmt"
	udpa_type_v1 "github.com/cncf/udpa/go/udpa/type/v1"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	gogoproto "github.com/gogo/protobuf/proto"
	goproto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	pany "github.com/golang/protobuf/ptypes/any"
	structpb "github.com/golang/protobuf/ptypes/struct"
)

func MessageToAny(msg goproto.Message) (*pany.Any, error) {

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

func MustMessageToAny(msg goproto.Message) *pany.Any {
	anymsg, err := MessageToAny(msg)
	if err != nil {
		panic(err)
	}
	return anymsg
}

func AnyToMessage(a *pany.Any) (goproto.Message, error) {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	return x.Message, err
}

func MustAnyToMessage(a *pany.Any) goproto.Message {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	if err != nil {
		panic(err)
	}
	return x.Message
}

// gogoprotos converted directly to goproto any can't be marshalled unless you wrap
// the contents of the gogoproto in a typed struct
func MustGogoMessageToAnyGoProto(msg goproto.Message) *pany.Any {
	any, err := GogoMessageToAnyGoProto(msg)
	if err != nil {
		panic(err)
	}
	return any
}

// gogoprotos converted directly to goproto any can't be marshalled unless you wrap
// the contents of the gogoproto in a typed struct
func GogoMessageToAnyGoProto(msg goproto.Message) (*pany.Any, error) {
	configStruct, err := conversion.MessageToStruct(msg)
	if err != nil {
		return nil, err
	}

	anyGogo := MustMessageToAny(msg)

	// create a typed struct so go proto can handle marshalling any types derived from gogo protos
	ts := &udpa_type_v1.TypedStruct{Value: configStruct, TypeUrl: anyGogo.TypeUrl}
	tsAnyGo := MustMessageToAny(ts)

	anyGo := &pany.Any{Value: tsAnyGo.Value, TypeUrl: tsAnyGo.TypeUrl}
	return anyGo, nil
}

// gogoproto any represented as a goproto can't be unmarshalled unless you unwrap
// the contents of the goproto from the typed struct (see function above)
// You may want to follow this with conversion.StructToMessage
func AnyGogoProtoToStructPb(a *pany.Any) (structpb.Struct, error) {
	msg, err := AnyToMessage(a)
	if err != nil {
		return structpb.Struct{}, err
	}
	ts := msg.(*udpa_type_v1.TypedStruct)

	configStruct := ts.GetValue()
	return *configStruct, nil
}

func protoToMessageName(msg goproto.Message) (string, error) {
	typeUrlPrefix := "type.googleapis.com/"

	if s := gogoproto.MessageName(msg); s != "" {
		return typeUrlPrefix + s, nil
	} else if s := goproto.MessageName(msg); s != "" {
		return typeUrlPrefix + s, nil
	}
	return "", fmt.Errorf("can't determine message name")
}

func protoToMessageBytes(msg goproto.Message) ([]byte, error) {
	if b, err := protoToMessageBytesGolang(msg); err == nil {
		return b, nil
	}
	return protoToMessageBytesGogo(msg)
}

func protoToMessageBytesGogo(msg goproto.Message) ([]byte, error) {
	b := gogoproto.NewBuffer(nil)
	b.SetDeterministic(true)
	err := b.Marshal(msg)
	return b.Bytes(), err
}

func protoToMessageBytesGolang(msg goproto.Message) ([]byte, error) {
	b := goproto.NewBuffer(nil)
	b.SetDeterministic(true)
	err := b.Marshal(msg)
	return b.Bytes(), err
}

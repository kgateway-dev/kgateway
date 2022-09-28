package utils

import (
	"context"
	"fmt"

	"github.com/solo-io/go-utils/contextutils"

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

// This will only DPanic in dev builds
func MustMessageToAny(msg proto.Message) *pany.Any {
	anymsg, err := MessageToAny(msg)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return &pany.Any{}
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
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return proto.MessageV1("")
	}
	return x.Message
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

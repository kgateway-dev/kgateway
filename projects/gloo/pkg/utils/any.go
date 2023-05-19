package utils

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	gogoproto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	pany "github.com/golang/protobuf/ptypes/any"
)

// MessageToAny takes any given proto message msg and returns the marshalled bytes of the proto, and a url to the type
// definition for the proto in the form of a *pany.Any, errors if nil or if the proto type doesnt exist or if there is
// a marshalling error
func MessageToAny(msg proto.Message) (*pany.Any, error) {
	if msg == nil {
		return nil, errors.New("MessageToAny: message cannot be nil")
	}
	name, err := protoToMessageName(msg)
	if err != nil {
		return nil, err
	}
	// Marshalls the message into bytes using the proto library, or gogoproto if proto errors
	buf, err := protoToMessageBytes(msg)
	if err != nil {
		return nil, err
	}
	return &pany.Any{
		TypeUrl: name,
		Value:   buf,
	}, nil
}

func AnyToMessage(a *pany.Any) (proto.Message, error) {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	return x.Message, err
}

// Deprecated: Use AnyToMessage
func MustAnyToMessage(a *pany.Any) proto.Message {
	var x ptypes.DynamicAny
	err := ptypes.UnmarshalAny(a, &x)
	if err != nil {
		panic(err)
	}
	return x.Message
}

func protoToMessageName(msg proto.Message) (string, error) {
	typeUrlPrefix := "type.googleapis.com/"

	potentialName := gogoproto.MessageName(msg)
	if potentialName != "" {
		return typeUrlPrefix + potentialName, nil
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

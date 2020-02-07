package pluginutils

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	pany "github.com/golang/protobuf/ptypes/any"
)

func MessageToAny(msg proto.Message) (*pany.Any, error) {
	b := proto.NewBuffer(nil)
	b.SetDeterministic(true)
	if err := b.Marshal(msg); err != nil {
		return nil, err
	}
	return &pany.Any{
		TypeUrl: "type.googleapis.com/" + proto.MessageName(msg),
		Value:   b.Bytes(),
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

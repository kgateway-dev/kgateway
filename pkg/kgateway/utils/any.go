package utils

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// MessageToAny takes any given proto message msg and returns the marshalled bytes of the proto, and a url to the type
// definition for the proto in the form of a *pany.Any, errors if nil or if the proto type doesnt exist or if there is
// a marshalling error
func MessageToAny(msg proto.Message) (*anypb.Any, error) {
	anyPb := &anypb.Any{}
	err := anypb.MarshalFrom(anyPb, msg, proto.MarshalOptions{
		Deterministic: true,
	})
	return anyPb, err
}

func MustMessageToAny(msg proto.Message) *anypb.Any {
	anyPb, err := MessageToAny(msg)
	if err != nil {
		panic(err)
	}
	return anyPb
}

func AnyToMessage(a *anypb.Any) (proto.Message, error) {
	return anypb.UnmarshalNew(a, proto.UnmarshalOptions{})
}

package utils

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

// AnyToJson converts an anypb.Any containing a google.protobuf.StringValue
// (which itself contains a JSON string) into a Go map or object.
func AnyToJson(anyVal *anypb.Any) (any, error) {
	if anyVal == nil {
		return nil, nil
	}

	sv := &wrapperspb.StringValue{}
	if err := anyVal.UnmarshalTo(sv); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Any to StringValue: %w", err)
	}

	var result any
	if err := json.Unmarshal([]byte(sv.GetValue()), &result); err != nil {
		return nil, fmt.Errorf("failed to parse internal JSON string: %w", err)
	}

	return result, nil
}

// JsonToAny converts a Go object (map, slice, etc.) back into a JSON string
// and wraps it in a google.protobuf.Any message.
func JsonToAny(obj any) (*anypb.Any, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	sv := wrapperspb.String(string(jsonBytes))

	anyVal, err := MessageToAny(sv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Any message: %w", err)
	}

	return anyVal, nil
}

package protoutils

import (
	"errors"
	"io"

	"github.com/ghodss/yaml"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var (
	jsonpbMarshaler               = &protojson.MarshalOptions{UseProtoNames: false}
	jsonpbMarshalerEmitZeroValues = &protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: true}
	jsonpbMarshalerIndented       = &protojson.MarshalOptions{UseProtoNames: false, Indent: "  "}
	jsonpbUnmarshalerAllowUnknown = &protojson.UnmarshalOptions{DiscardUnknown: true}

	NilStructError = errors.New("cannot unmarshal nil struct")
)

// this function is designed for converting go object (that is not a proto.Message) into a
// pb Struct, based on json struct tags
func MarshalStruct(m proto.Message) (*structpb.Struct, error) {
	data, err := MarshalBytes(m)
	if err != nil {
		return nil, err
	}
	var pb structpb.Struct
	err = protojson.Unmarshal(data, &pb)
	return &pb, err
}

func MarshalStructEmitZeroValues(m proto.Message) (*structpb.Struct, error) {
	data, err := MarshalBytesEmitZeroValues(m)
	if err != nil {
		return nil, err
	}
	var pb structpb.Struct
	err = protojson.Unmarshal(data, &pb)
	return &pb, err
}

func MarshalBytes(pb proto.Message) ([]byte, error) {
	return jsonpbMarshaler.Marshal(pb)
}

func MarshalBytesIndented(pb proto.Message) ([]byte, error) {
	return jsonpbMarshalerIndented.Marshal(pb)
}

func MarshalBytesEmitZeroValues(pb proto.Message) ([]byte, error) {
	return jsonpbMarshalerEmitZeroValues.Marshal(pb)
}

func UnmarshalBytes(data []byte, into proto.Message) error {
	return protojson.Unmarshal(data, into)
}

func UnmarshalBytesAllowUnknown(data []byte, into proto.Message) error {
	return jsonpbUnmarshalerAllowUnknown.Unmarshal(data, into)
}

func UnmarshalAllowUnknown(r io.Reader, into proto.Message) error {
	data, _ := io.ReadAll(r)
	return UnmarshalBytesAllowUnknown(data, into)
}

func UnmarshalYaml(data []byte, into proto.Message) error {
	jsn, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(jsn, into)
}

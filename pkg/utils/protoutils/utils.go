package protoutils

import (
	"bytes"
	"io"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/rotisserie/eris"
)

var (
	jsonpbMarshaler               = &jsonpb.Marshaler{OrigName: false}
	jsonpbMarshalerEmitZeroValues = &jsonpb.Marshaler{OrigName: false, EmitDefaults: true}
	jsonpbMarshalerIndented       = &jsonpb.Marshaler{OrigName: false, Indent: "  "}
	jsonpbUnmarshalerAllowUnknown = &jsonpb.Unmarshaler{AllowUnknownFields: true}

	NilStructError = eris.New("cannot unmarshal nil struct")
)

// this function is designed for converting go object (that is not a proto.Message) into a
// pb Struct, based on json struct tags
func MarshalStruct(m proto.Message) (*structpb.Struct, error) {
	data, err := MarshalBytes(m)
	if err != nil {
		return nil, err
	}
	var pb structpb.Struct
	err = jsonpb.UnmarshalString(string(data), &pb)
	return &pb, err
}

func MarshalStructEmitZeroValues(m proto.Message) (*structpb.Struct, error) {
	data, err := MarshalBytesEmitZeroValues(m)
	if err != nil {
		return nil, err
	}
	var pb structpb.Struct
	err = jsonpb.UnmarshalString(string(data), &pb)
	return &pb, err
}

func MarshalBytes(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := jsonpbMarshaler.Marshal(buf, pb)
	return buf.Bytes(), err
}

func MarshalBytesIndented(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := jsonpbMarshalerIndented.Marshal(buf, pb)
	return buf.Bytes(), err
}

func MarshalBytesEmitZeroValues(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := jsonpbMarshalerEmitZeroValues.Marshal(buf, pb)
	return buf.Bytes(), err
}

func UnmarshalBytes(data []byte, into proto.Message) error {
	return jsonpb.Unmarshal(bytes.NewBuffer(data), into)
}

func UnmarshalBytesAllowUnknown(data []byte, into proto.Message) error {
	return UnmarshalAllowUnknown(bytes.NewBuffer(data), into)
}

func UnmarshalAllowUnknown(r io.Reader, into proto.Message) error {
	return jsonpbUnmarshalerAllowUnknown.Unmarshal(r, into)
}

func UnmarshalYaml(data []byte, into proto.Message) error {
	jsn, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	return jsonpb.Unmarshal(bytes.NewBuffer(jsn), into)
}

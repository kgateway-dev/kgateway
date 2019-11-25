package protoutils

import (
	"bytes"
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/go-utils/errors"
)

var jsonpbMarshaler = &jsonpb.Marshaler{OrigName: false}
var jsonpbMarshalerEmitZeroValues = &jsonpb.Marshaler{OrigName: false, EmitDefaults: true}

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

func UnmarshalStruct(structuredData *structpb.Struct, into interface{}) error {
	if structuredData == nil {
		return errors.New("cannot unmarshal nil proto struct")
	}
	strData, err := jsonpbMarshaler.MarshalToString(structuredData)
	if err != nil {
		return err
	}
	data := []byte(strData)
	return json.Unmarshal(data, into)
}

func MarshalBytes(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := jsonpbMarshaler.Marshal(buf, pb)
	return buf.Bytes(), err
}

func MarshalBytesEmitZeroValues(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := jsonpbMarshalerEmitZeroValues.Marshal(buf, pb)
	return buf.Bytes(), err
}

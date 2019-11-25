package protoutils

import (
	"bytes"
	"encoding/json"

	gogojson "github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/go-utils/errors"
)

var (
	jsonpbMarshaler               = &jsonpb.Marshaler{OrigName: false}
	jsonpbMarshalerEmitZeroValues = &jsonpb.Marshaler{OrigName: false, EmitDefaults: true}

	gogoJsonpbMarshaler = &gogojson.Marshaler{OrigName: false}
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

func StructPbToGogo(structuredData *structpb.Struct) (*types.Struct, error) {
	if structuredData == nil {
		return nil, errors.New("cannot unmarshal nil proto struct")
	}
	buf := &bytes.Buffer{}
	if err := jsonpbMarshaler.Marshal(buf, structuredData); err != nil {
		return nil, err
	}
	var st types.Struct
	if err := gogojson.Unmarshal(buf, &st); err != nil {
		return nil, err
	}

	return &st, nil
}

func StructGogoToPb(structuredData *types.Struct) (*structpb.Struct, error) {
	if structuredData == nil {
		return nil, errors.New("cannot unmarshal nil proto struct")
	}
	buf := &bytes.Buffer{}
	if err := gogoJsonpbMarshaler.Marshal(buf, structuredData); err != nil {
		return nil, err
	}
	var st structpb.Struct
	if err := jsonpb.Unmarshal(buf, &st); err != nil {
		return nil, err
	}

	return &st, nil
}

func AnyPbToGogo(structuredData *any.Any) (*types.Any, error) {
	if structuredData == nil {
		return nil, errors.New("cannot unmarshal nil proto struct")
	}
	return &types.Any{
		TypeUrl: structuredData.GetTypeUrl(),
		Value:   structuredData.GetValue(),
	}, nil
}

func AnyGogoToPb(structuredData *types.Any) (*any.Any, error) {
	if structuredData == nil {
		return nil, errors.New("cannot unmarshal nil proto struct")
	}
	return &any.Any{
		TypeUrl: structuredData.GetTypeUrl(),
		Value:   structuredData.GetValue(),
	}, nil
}

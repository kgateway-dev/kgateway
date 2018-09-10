package protoutil

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
)

var jsonpbMarshaler = &jsonpb.Marshaler{OrigName: true}

// this function is designed for converting go object (that is not a proto.Message) into a
// pb Struct, based on json struct tags
func MarshalStruct(m interface{}) (*types.Struct, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var pb types.Struct
	err = jsonpb.UnmarshalString(string(data), &pb)
	return &pb, err
}

func UnmarshalStructToProto(structuredData *types.Struct, into proto.Message) error {
	if structuredData == nil {
		return errors.New("cannot unmarshal nil proto struct")
	}
	strData, err := jsonpbMarshaler.MarshalToString(structuredData)
	if err != nil {
		return err
	}
	data := []byte(strData)
	return Unmarshal(data, into)
}
func UnmarshalProtoToStruct(m proto.Message) (*types.Struct, error) {
	strData, err := jsonpbMarshaler.MarshalToString(m)
	if err != nil {
		return nil, err
	}
	var pb types.Struct
	err = jsonpb.UnmarshalString(strData, &pb)
	return &pb, err
}

func UnmarshalStruct(structuredData *types.Struct, into interface{}) error {
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

func UnmarshalValue(value *types.Value, into interface{}) error {
	switch kind := value.Kind.(type) {
	case *types.Value_StructValue:
		return UnmarshalStruct(kind.StructValue, into)
	}
	return errors.Errorf("cannot call UnmarshalValue on non-struct values")
}

func Unmarshal(data []byte, into proto.Message) error {
	return jsonpb.Unmarshal(bytes.NewBuffer(data), into)
}

func Marshal(pb proto.Message) ([]byte, error) {
	buf := &bytes.Buffer{}
	err := jsonpbMarshaler.Marshal(buf, pb)
	return buf.Bytes(), err
}

func MarshalMap(from proto.Message) (map[string]interface{}, error) {
	data, err := Marshal(from)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	err = json.Unmarshal(data, &m)
	return m, err
}

func UnmarshalMap(m map[string]interface{}, into proto.Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return Unmarshal(data, into)
}

package protoutils

import (
	"reflect"
	"strings"

	proto2 "github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/proto"
	"github.com/rotisserie/eris"
)

type MultiAnyResolver struct{}

func (m *MultiAnyResolver) Resolve(typeUrl string) (proto.Message, error) {
	// Mostly copy pasted from: https://github.com/golang/protobuf/blob/84668698ea25b64748563aa20726db66a6b8d299/jsonpb/jsonpb.go#L93
	messageType := typeUrl
	if slash := strings.LastIndex(typeUrl, "/"); slash >= 0 {
		messageType = messageType[slash+1:]
	}
	var mt reflect.Type
	mt = proto.MessageType(messageType)
	if mt == nil {
		// If no any exists for golang proto, check gogo proto
		mt = proto2.MessageType(messageType)
		if mt == nil {
			// If neither exists, there is an error
			return nil, eris.Errorf("unknown message type %q", messageType)
		}
	}
	return reflect.New(mt.Elem()).Interface().(proto.Message), nil
}

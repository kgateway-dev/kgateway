package syncutil

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/mitchellh/reflectwalk"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const (
	Redacted = "[REDACTED]"

	LogRedactorTag           = "logging"
	LogRedactorTagValue      = "redact"
	SerializationFieldPrefix = "XXX"
)

// stringify the contents of the snapshot
//
// NOTE that if any of the top-level fields of the snapshot is a SecretList, then the secrets will be
// stringified by printing just their name and namespace, and "REDACTED" for their data. Secrets may
// contain sensitive data like TLS private keys, so be sure to use this whenever you'd like to
// stringify a snapshot rather than Go's %v formatter
func StringifySnapshot(snapshot interface{}) string {
	snapshotStruct := reflect.ValueOf(snapshot).Elem()
	stringBuilder := strings.Builder{}

	for i := 0; i < snapshotStruct.NumField(); i++ {
		fieldName := snapshotStruct.Type().Field(i).Name
		fieldValue := snapshotStruct.Field(i).Interface()

		stringBuilder.Write([]byte(fieldName))
		stringBuilder.Write([]byte(":"))

		if secretList, ok := fieldValue.(v1.SecretList); ok {
			stringBuilder.Write([]byte("["))

			var redactedSecrets []string
			secretList.Each(func(s *v1.Secret) {
				redactedSecret := fmt.Sprintf(
					"%v{name: %s namespace: %s data: %s}",
					reflect.TypeOf(s),
					s.Metadata.Name,
					s.Metadata.Namespace,
					Redacted,
				)

				redactedSecrets = append(redactedSecrets, redactedSecret)
			})

			stringBuilder.Write([]byte(strings.Join(redactedSecrets, " '")))
			stringBuilder.Write([]byte("]"))
		} else {
			stringBuilder.Write([]byte(fmt.Sprintf("%v", fieldValue)))
		}
		stringBuilder.Write([]byte("\n"))
	}

	return stringBuilder.String()
}

type ProtoRedactor interface {
	// Build a JSON string representation of the proto message, zeroing-out all fields in the proto that match some criteria
	BuildRedactedJsonString(message proto.Message) (string, error)
}

// build a ProtoRedactor that zeroes out fields that have the given struct tag set to the given value
func NewProtoRedactor(tagName, tagValue string) ProtoRedactor {
	return &protoRedactor{
		tagName:  tagName,
		tagValue: tagValue,
	}
}

type protoRedactor struct {
	tagName  string
	tagValue string
}

func (p *protoRedactor) BuildRedactedJsonString(message proto.Message) (string, error) {
	// make a clone so that we can mutate it and zero-out fields
	clone := proto.Clone(message)

	walker := &structWalker{
		tagName:  p.tagName,
		tagValue: p.tagValue,
	}
	err := reflectwalk.Walk(clone, walker)
	if err != nil {
		return "", err
	}

	bytes, err := json.Marshal(clone)
	return string(bytes), err
}

// run the StructField callback for every field in the proto
type structWalker struct {
	tagName  string
	tagValue string
}

func (s *structWalker) Struct(reflect.Value) error {
	return nil
}

func (s *structWalker) StructField(field reflect.StructField, value reflect.Value) error {
	if field.Tag.Get(s.tagName) == s.tagValue {
		value.Set(reflect.Zero(value.Type()))
	}
	return nil
}

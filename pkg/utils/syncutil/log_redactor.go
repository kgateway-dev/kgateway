package syncutil

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/protoc-gen-ext/pkg/redaction"
)

const (
	Redacted = "[REDACTED]"
)

// stringify the contents of the snapshot
//
// NOTE that if any of the top-level fields of the snapshot is a SecretList or ArtifactList,
// then the values from those lists will be stringified by printing just their name and namespace,
// and "REDACTED" for their data. Secrets or Artifacts may contain sensitive data like TLS private keys,
// so be sure to use this whenever you'd like to stringify a snapshot rather than Go's %v formatter
func StringifySnapshot(snapshot interface{}) string {
	snapshotStruct := reflect.ValueOf(snapshot).Elem()
	stringBuilder := strings.Builder{}

	for i := 0; i < snapshotStruct.NumField(); i++ {
		fieldName := snapshotStruct.Type().Field(i).Name
		fieldValue := snapshotStruct.Field(i).Interface()

		stringBuilder.Write([]byte(fieldName))
		stringBuilder.Write([]byte(":"))

		// Redact Secrets, which contain sensitive data
		if secretList, ok := fieldValue.(v1.SecretList); ok {
			writeRedactedResourceList(&stringBuilder, secretList.AsResources())
			stringBuilder.Write([]byte("\n"))
			continue
		}

		// Redact Artifacts, which may contain sensitive data
		// Sensitive data should be stored in secrets, but to be extra safe, we redact Artifact content as well
		if artifactList, ok := fieldValue.(v1.ArtifactList); ok {
			writeRedactedResourceList(&stringBuilder, artifactList.AsResources())
			stringBuilder.Write([]byte("\n"))
			continue
		}

		stringBuilder.Write([]byte(fmt.Sprintf("%v", fieldValue)))
		stringBuilder.Write([]byte("\n"))
	}

	return stringBuilder.String()
}

func writeRedactedResourceList(stringBuilder *strings.Builder, resourceList resources.ResourceList) {
	stringBuilder.Write([]byte("["))

	var redactedResources []string
	resourceList.Each(func(r resources.Resource) {
		redactedResource := fmt.Sprintf(
			"%v{name: %s namespace: %s data: %s}",
			reflect.TypeOf(r),
			r.GetMetadata().GetName(),
			r.GetMetadata().GetNamespace(),
			Redacted,
		)

		redactedResources = append(redactedResources, redactedResource)
	})

	stringBuilder.Write([]byte(strings.Join(redactedResources, " '")))
	stringBuilder.Write([]byte("]"))
}

type ProtoRedactor interface {
	// Build a JSON string representation of the proto message, zeroing-out all fields in the proto that match some criteria
	BuildRedactedJsonString(message proto.Message) (string, error)
}

// build a ProtoRedactor that zeroes out fields that have the given struct tag set to the given value
func NewProtoRedactor() ProtoRedactor {
	return &protoRedactor{}
}

type protoRedactor struct{}

func (p *protoRedactor) BuildRedactedJsonString(message proto.Message) (string, error) {
	// make a clone so that we can mutate it and zero-out fields
	clone := proto.Clone(message)

	redaction.Redact(proto.MessageReflect(clone))

	bytes, err := json.Marshal(clone)
	return string(bytes), err
}

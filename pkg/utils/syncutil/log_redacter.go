package syncutil

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	xdsproto "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
)

const (
	Redacted = "[REDACTED]"

	TagName                  = "logging"
	TagValue                 = "redact"
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

type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// Convert all the configs into JSON, but without XXX_ serialization fields and without struct fields tagged with "logging: redact", then log them in that form
func LogRedactedExtAuthConfigs(logger Logger, configs []*xdsproto.ExtAuthConfig) {
	logger.Infof("Processing %d new configs", len(configs))
	for _, authConfig := range configs {
		for _, authConfigSpec := range authConfig.Configs {
			redactedJson := redactObject(authConfigSpec)
			jsonBytes, err := json.Marshal(redactedJson)
			if err == nil {
				logger.Infof("New config: %s", string(jsonBytes))
			} else {
				logger.Errorf("Failed to convert auth config into redacted JSON for logging: %+v", err)
			}
		}
	}
}

func redactObject(o interface{}) map[string]interface{} {
	if reflect.ValueOf(o).IsNil() {
		return nil
	}
	jsonRepresentation := map[string]interface{}{}
	if o == struct{}{} {
		return jsonRepresentation
	}
	elem := reflect.ValueOf(o).Elem()
	for i := 0; i < elem.NumField(); i++ {
		fieldName := elem.Type().Field(i).Name

		if strings.HasPrefix(fieldName, SerializationFieldPrefix) {
			continue
		}

		fieldValue := elem.Field(i).Interface()
		kind := elem.Field(i).Kind()

		tagValue := elem.Type().Field(i).Tag.Get(TagName)
		if kind == reflect.Struct || kind == reflect.Interface || kind == reflect.Ptr {
			jsonRepresentation[fieldName] = redactObject(fieldValue)
		} else if tagValue == TagValue {
			jsonRepresentation[fieldName] = Redacted
		} else {
			jsonRepresentation[fieldName] = fieldValue
		}
	}

	return jsonRepresentation
}

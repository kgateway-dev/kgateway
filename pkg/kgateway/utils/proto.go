package utils

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
)

// DurationToProto converts a go Duration to a protobuf Duration.
func DurationToProto(d time.Duration) *durationpb.Duration {
	return &durationpb.Duration{
		Seconds: int64(d) / int64(time.Second),
		Nanos:   int32(int64(d) % int64(time.Second)), //nolint:gosec // G115: nanoseconds modulo 1 second always fits in int32
	}
}

// JSONToProtoStruct converts raw JSON data from a Kubernetes API resource to a protobuf Struct
func JSONToProtoStruct(jsonExtension []byte) (*structpb.Struct, error) {
	if jsonExtension == nil {
		return nil, nil
	}

	var formatMap map[string]any
	if err := json.Unmarshal(jsonExtension, &formatMap); err != nil {
		return nil, fmt.Errorf("invalid json object: %w", err)
	}

	structVal, err := structpb.NewStruct(formatMap)
	if err != nil {
		return nil, fmt.Errorf("invalid json object: %w", err)
	}

	return structVal, nil
}

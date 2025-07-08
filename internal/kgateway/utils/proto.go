package utils

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
)

// DurationToProto converts a go Duration to a protobuf Duration.
func DurationToProto(d time.Duration) *durationpb.Duration {
	return &durationpb.Duration{
		Seconds: int64(d) / int64(time.Second),
		Nanos:   int32(int64(d) % int64(time.Second)),
	}
}

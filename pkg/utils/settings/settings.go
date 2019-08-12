package settings

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type settingsKeyStruct struct{}

var settingsKey = settingsKeyStruct{}

func WithSettings(ctx context.Context, settings *v1.Settings) context.Context {
	return context.WithValue(ctx, settingsKey, settings)
}

func FromContext(ctx context.Context) *v1.Settings {
	if ctx != nil {
		if settings, ok := ctx.Value(settingsKey).(*v1.Settings); ok {
			return settings
		}
	}
	return nil
}

func IsAllNamespaces(s *v1.Settings) bool {
	if s == nil {
		return false
	}
	return len(s.WatchNamespaces) == 0
}

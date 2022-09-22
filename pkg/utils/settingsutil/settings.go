package settingsutil

import (
	"context"

	"github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/contextutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type settingsKeyStruct struct{}

var settingsKey = settingsKeyStruct{}

func WithSettings(ctx context.Context, settings *v1.Settings) context.Context {
	return context.WithValue(ctx, settingsKey, settings)
}

func FromContext(ctx context.Context) (*v1.Settings, error) {
	if ctx == nil {
		return nil, nil
	}
	settings := MaybeFromContext(ctx)
	if settings != nil {
		return settings, nil
	}
	// we should always have settings when this method is called.
	contextutils.LoggerFrom(ctx).DPanic("no settings on context")
	return nil, eris.Errorf("no settings on context")
}

func MaybeFromContext(ctx context.Context) *v1.Settings {
	if ctx == nil {
		return nil
	}
	if settings, ok := ctx.Value(settingsKey).(*v1.Settings); ok {
		return settings
	}
	return nil
}

func IsAllNamespacesFromSettings(s *v1.Settings) bool {
	if s == nil {
		return false
	}
	return IsAllNamespaces(s.GetWatchNamespaces())
}

func IsAllNamespaces(watchNs []string) bool {
	switch {
	case len(watchNs) == 0:
		return true
	case len(watchNs) == 1 && watchNs[0] == "":
		return true
	default:
		return false
	}
}

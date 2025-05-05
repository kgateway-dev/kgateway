package logging

import (
	"log/slog"
	"sync"
)

const (
	DefaultComponent = "default"
)

// componentLeveler maps component names to their respective slog.LevelVar instance
var componentLeveler sync.Map

func init() {
	defaultLogger := New(DefaultComponent)
	slog.SetDefault(defaultLogger)
}

// New returns a new slog.Logger instance for the given component with default Options.
// If the component is empty, it returns the default logger.
func New(component string) *slog.Logger {
	return NewWithOptions(component, Options{})
}

// NewWithOptions returns a new slog.Logger instance for the given component with the provided Options
// If the component is empty, it returns the default logger.
func NewWithOptions(component string, opts Options) *slog.Logger {
	if component == "" {
		return slog.Default()
	}

	opts.Default()

	level := &slog.LevelVar{} // default is INFO
	level.Set(opts.Level)

	handlerOpts := &slog.HandlerOptions{
		AddSource:   opts.AddSource,
		Level:       level,
		ReplaceAttr: slogLevelReplacer,
	}

	componentLeveler.Store(component, level)
	var slogHandler slog.Handler
	switch opts.Format {
	case TextFormat:
		slogHandler = slog.NewTextHandler(opts.Writer, handlerOpts)
	case JSONFormat:
		slogHandler = slog.NewJSONHandler(opts.Writer, handlerOpts)
	default:
		slogHandler = slog.NewTextHandler(opts.Writer, handlerOpts)
	}

	return slog.New(slogHandler)
}

// need a function to get map of component names to level
func GetComponentLevels() map[string]slog.Level {
	levels := make(map[string]slog.Level)
	componentLeveler.Range(func(key any, value any) bool {
		levels[key.(string)] = value.(*slog.LevelVar).Level()
		return true
	})
	return levels
}

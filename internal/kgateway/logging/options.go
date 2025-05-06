package logging

import (
	"io"
	"log/slog"
	"os"
)

// Options to configure the logger
type Options struct {
	// Logger level
	Level slog.Level

	// Log format: text or json
	Format LogFormat

	// Writer to write logs to
	Writer io.Writer

	// AddSource adds the source code position of the log statement to the output
	AddSource bool
}

// LogFormat represents the format of the log output
type LogFormat string

const (
	// TextFormat represents plain text format
	TextFormat LogFormat = "text"

	// JSONFormat represents JSON format
	JSONFormat LogFormat = "json"
)

// Default sets default values on Options
func (o *Options) Default() {
	// Level implicitly defaults to INFO
	if o.Format == "" {
		o.Format = JSONFormat
	}
	if o.Writer == nil {
		o.Writer = os.Stderr
	}
}

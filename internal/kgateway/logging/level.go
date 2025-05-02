package logging

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

var (
	GlobalLevel = &slog.LevelVar{} // default is INFO
)

func SetLogLevel(level string) {
	slogLevel, err := parseLevel(level)
	if err != nil {
		slog.Error("failed to parse log level", "error", err)
		return
	}
	GlobalLevel.Set(slogLevel)
}

func SlogLevelReplacer(groups []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.LevelKey {
		level := attr.Value.Any().(slog.Level)
		levelname := level.String()
		attr.Value = slog.StringValue(levelname)
	}
	return attr
}

func SetLogLevelHandler(w http.ResponseWriter, r *http.Request) {
	levelParam := r.URL.Query().Get("level")
	if levelParam == "" {
		w.WriteHeader(http.StatusOK)
		info := fmt.Sprintf(`Current log level: %s

To update, send a POST request with query parameter level=<error|warn|info|debug|trace>`,
			strings.ToLower(GlobalLevel.Level().String()))
		w.Write([]byte(info)) // nolint: errcheck
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method must be one of POST|PUT", http.StatusMethodNotAllowed)
		return
	}
	newLevel, err := parseLevel(levelParam)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error())) // nolint: errcheck
		return
	}

	GlobalLevel.Set(newLevel)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("log level set to: %s", strings.ToLower(newLevel.String())))) // nolint: errcheck
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %s; should be one of error|warn|info|debug|trace", level)
	}
}

package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// A Level is a logging priority. Higher levels are more important.
type LogLevel int8

const (
	LogLevelInfo LogLevel = iota - 1
	LogLevelWarn
	LogLevelError
	LogLevelAll
)

// String returns a lower-case ASCII representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	case LogLevelAll:
		return "all"
	default:
		return fmt.Sprintf("Level(%d)", l)
	}
}

// Filter JSON logs with level
func FilterLogLevel(r io.ReadCloser, level LogLevel) strings.Builder {
	scanner := bufio.NewScanner(r)
	logs := strings.Builder{}
	for scanner.Scan() {
		line := scanner.Text()
		start := strings.Index(line, "{")
		if start == -1 {
			continue
		}
		in := []byte(line[start:])
		var raw map[string]interface{}
		if err := json.Unmarshal(in, &raw); err != nil {
			continue
		}
		if raw["level"] == level.String() || level == LogLevelAll {
			logs.WriteString(line + "\n")
		}
	}
	return logs
}

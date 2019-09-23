package utils

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// Filter JSON logs with level
// if "all" is passed in as a level, then all levels are allowed
func FilterLogLevel(r io.ReadCloser, level string) strings.Builder {
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
		if raw["level"] == level || level == "all" {
			logs.WriteString(line + "\n")
		}
	}
	return logs
}

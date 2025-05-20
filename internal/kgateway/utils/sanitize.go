package utils

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Virtual host and virtual cluster names cannot contain dots, otherwise Envoy might incorrectly compute
// its statistics tree. Any occurrences will be replaced with underscores.
const (
	illegalChar     = "."
	replacementChar = "_"
)

func SanitizeForEnvoy(ctx context.Context, resourceName, resourceTypeName string) string {
	if strings.Contains(resourceName, illegalChar) {
		//nolint:sloglint // ignore formatting
		slog.Debug(fmt.Sprintf("illegal character(s) '%s' in %s name [%s] will be replaced by '%s'",
			illegalChar, resourceTypeName, resourceName, replacementChar))
		resourceName = strings.ReplaceAll(resourceName, illegalChar, replacementChar)
	}
	return resourceName
}

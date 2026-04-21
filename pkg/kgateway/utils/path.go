package utils

import (
	"strings"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// NormalizePathMatch applies Gateway API PathPrefix semantics before downstream comparisons
// and xDS translation. Per spec, a trailing slash on a PathPrefix value is ignored.
func NormalizePathMatch(pathType gwv1.PathMatchType, pathValue string) string {
	if pathType != gwv1.PathMatchPathPrefix || pathValue == "/" {
		return pathValue
	}

	normalized := strings.TrimRight(pathValue, "/")
	if normalized == "" {
		return "/"
	}

	return normalized
}

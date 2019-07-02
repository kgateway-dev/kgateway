package utils

import "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

func ResourceRefPtr(ref core.ResourceRef) *core.ResourceRef {
	return &ref
}

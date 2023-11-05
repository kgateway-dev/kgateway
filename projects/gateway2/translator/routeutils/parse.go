package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ParsePath(path *gwv1.HTTPPathMatch) (gwv1.PathMatchType, string) {
	pathType := gwv1.PathMatchPathPrefix
	pathValue := "/"
	if path != nil && path.Type != nil {
		pathType = *path.Type
	}
	if path != nil && path.Value != nil {
		pathValue = *path.Value
	}
	return pathType, pathValue
}

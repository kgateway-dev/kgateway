package filterplugins

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type FilterPlugin interface {
	// outputRoute.Options is guaranteed to be non-nil
	ApplyFilter(
		ctx context.Context,
		filter gwv1.HTTPRouteFilter,
		outputRoute *v1.Route,
	) error
}

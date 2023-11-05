package filterplugins

import (
	"context"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type FilterPlugin interface {
	// outputRoute.Options is guaranteed to be non-nil
	ApplyFilter(
		ctx context.Context,
		filter gwv1.HTTPRouteFilter,
		outputRoute *routev3.Route,
	) error
}

package sanitizer

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type InvalidUpstreamRemovingSanitizer struct{}

func NewInvalidUpstreamRemovingSanitizer() *InvalidUpstreamRemovingSanitizer {
	return &InvalidUpstreamRemovingSanitizer{}
}

// If there are any errors on upstreams, this function tries to remove the correspondent clusters and endpoints from
// the xDS snapshot. If the snapshot is still consistent after these mutations and there are no errors related to other
// resources, we are good to send it to Envoy.
//
func (s *InvalidUpstreamRemovingSanitizer) SanitizeSnapshot(ctx context.Context, glooSnapshot *v1.ApiSnapshot, xdsSnapshot envoycache.Snapshot, reports reporter.ResourceReports) (envoycache.Snapshot, error) {
	ctx = contextutils.WithLogger(ctx, "invalid-upstream-remover")

	resourcesErr := reports.ValidateStrict()
	if resourcesErr == nil {
		return xdsSnapshot, nil
	}

	contextutils.LoggerFrom(ctx).Debug("removing errored upstreams and checking consistency")

	clusters := xdsSnapshot.GetResources(xds.ClusterType)
	endpoints := xdsSnapshot.GetResources(xds.EndpointType)

	// Find all the errored upstreams and remove them from the xDS snapshot
	for _, up := range glooSnapshot.Upstreams.AsInputResources() {
		if reports[up].Errors != nil {
			clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
			// remove cluster and endpoints
			delete(clusters.Items, clusterName)
			delete(endpoints.Items, clusterName)
		}
	}

	// TODO(marco): the function accepts and return a Snapshot interface, but then swaps in its own implementation.
	//  This breaks the abstraction and mocking the snapshot becomes impossible. We should have a generic way of
	//  creating snapshots.
	xdsSnapshot = xds.NewSnapshotFromResources(
		endpoints,
		clusters,
		xdsSnapshot.GetResources(xds.RouteType),
		xdsSnapshot.GetResources(xds.ListenerType),
	)

	// If the snapshot is not consistent,
	if xdsSnapshot.Consistent() != nil {
		return xdsSnapshot, resourcesErr
	}

	// Snapshot is consistent, so check if we have errors not related to the upstreams
	resourcesErr = reports.Validate()

	return xdsSnapshot, resourcesErr
}

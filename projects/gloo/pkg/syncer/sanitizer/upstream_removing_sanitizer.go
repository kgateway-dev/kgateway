package sanitizer

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/pkg/utils"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	mUpstreamsRemoved = utils.MakeLastValueCounter("gloo.solo.io/sanitizer/upstreams_removed", "The number upstreams removed from the sanitized xds snapshot", stats.ProxyNameKey)

	// Compile-time assertion
	_ XdsSanitizer = new(UpstreamRemovingSanitizer)
)

type UpstreamRemovingSanitizer struct {
	// note to devs: this can be called in parallel by the validation webhook and main translation loops at the same time
	// any stateful fields should be protected by a mutex
}

func NewUpstreamRemovingSanitizer() *UpstreamRemovingSanitizer {
	return &UpstreamRemovingSanitizer{}
}

// If there are any errors on upstreams, this function tries to remove the correspondent clusters and endpoints from
// the xDS snapshot. If the snapshot is still consistent after these mutations and there are no errors related to other
// resources, we are good to send it to Envoy.
func (s *UpstreamRemovingSanitizer) SanitizeSnapshot(
	ctx context.Context,
	glooSnapshot *v1snap.ApiSnapshot,
	xdsSnapshot envoycache.Snapshot,
	reports reporter.ResourceReports,
) envoycache.Snapshot {
	ctx = contextutils.WithLogger(ctx, "invalid-upstream-remover")

	if reports.Validate() == nil {
		return xdsSnapshot
	}

	contextutils.LoggerFrom(ctx).Debug("removing errored upstreams and checking consistency")

	clusters := xdsSnapshot.GetResources(types.ClusterTypeV3)
	endpoints := xdsSnapshot.GetResources(types.EndpointTypeV3)
	var removed int64

	// Find all the errored upstreams and remove them from the xDS snapshot
	for _, up := range glooSnapshot.Upstreams.AsInputResources() {

		if reports[up].Errors != nil {

			clusterName := translator.UpstreamToClusterName(up.GetMetadata().Ref())
			if clusters.Items[clusterName] == nil {
				// cluster has already been removed from the snapshot
				contextutils.LoggerFrom(ctx).Debugf("cluster %s does not exist in the xds snapshot", clusterName)
				continue
			}
			endpointName := clusterName
			cluster, _ := clusters.Items[clusterName].ResourceProto().(*envoy_config_cluster_v3.Cluster)
			if cluster.GetType() == envoy_config_cluster_v3.Cluster_EDS {
				if cluster.GetEdsClusterConfig().GetServiceName() != "" {
					endpointName = cluster.GetEdsClusterConfig().GetServiceName()
				} else {
					endpointName = cluster.GetName()
				}
			}
			// remove cluster and endpoints
			delete(clusters.Items, clusterName)
			delete(endpoints.Items, endpointName)
			removed++
		}
	}

	utils.Measure(ctx, mUpstreamsRemoved, removed)

	// TODO(marco): the function accepts and return a Snapshot interface, but then swaps in its own implementation.
	//  This breaks the abstraction and mocking the snapshot becomes impossible. We should have a generic way of
	//  creating snapshots.
	newXdsSnapshot := xds.NewSnapshotFromResources(
		endpoints,
		clusters,
		xdsSnapshot.GetResources(types.RouteTypeV3),
		xdsSnapshot.GetResources(types.ListenerTypeV3),
	)

	// Convert errors related to upstreams to warnings
	for _, up := range glooSnapshot.Upstreams.AsInputResources() {
		if upReport := reports[up]; upReport.Errors != nil {
			upReport.Warnings = []string{upReport.Errors.Error()}
			upReport.Errors = nil
			reports[up] = upReport
		}
	}

	return newXdsSnapshot
}

package xds

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type DiscoveryInputs struct {
	Clusters  []*clusterv3.Cluster
	Endpoints []*endpointv3.ClusterLoadAssignment
	Warnings  []string
}

type XdsSyncResult struct {
	ResourceReports reporter.ResourceReports
}

// ProyxSyncer is the write interface
// where different translators can publish
// their outputs (which are the proxy syncer inputs)
type ProyxSyncer interface {
	UpdateDiscoveryInputs(ctx context.Context, inputs DiscoveryInputs)
	UpdateSecretInputs(ctx context.Context)
	Kick(ctx context.Context)
}

type ProxyXdsSyncer interface {
	ProyxSyncer
	SyncXdsOnEvent(
		ctx context.Context,
		onXdsSynced func(XdsSyncResult),
	)
}

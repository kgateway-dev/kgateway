package xds

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type DiscoveryInputs struct {
	Upstreams v1.UpstreamList
	Endpoints v1.EndpointList
}

type SecretInputs struct {
	Secrets v1.SecretList
}

type XdsSyncResult struct {
	ResourceReports reporter.ResourceReports
}

// ProyxSyncer is the write interface
// where different translators can publish
// their outputs (which are the proxy syncer inputs)
type ProyxSyncer interface {
	UpdateDiscoveryInputs(ctx context.Context, inputs DiscoveryInputs)
	UpdateSecretInputs(ctx context.Context, inputs SecretInputs)
	Kick(ctx context.Context)
}

type ProxyXdsSyncer interface {
	ProyxSyncer
	SyncXdsOnEvent(
		ctx context.Context,
		onXdsSynced func(XdsSyncResult),
	)
}

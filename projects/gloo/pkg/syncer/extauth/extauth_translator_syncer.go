package extauth

import (
	"context"

	"github.com/rotisserie/eris"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	extauthConnectedStateDescription = "zero indicates gloo detected an error with the auth config and did not update its XDS snapshot, check the gloo logs for errors"
	extauthConnectedState            = stats.Int64("glooe.extauth/connected_state", extauthConnectedStateDescription, "1")

	extauthConnectedStateView = &view.View{
		Name:        "glooe.extauth/connected_state",
		Measure:     extauthConnectedState,
		Description: extauthConnectedStateDescription,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{},
	}
)

const (
	Name              = "extauth"
	ExtAuthServerRole = "extauth"
	ErrEnterpriseOnly = "The Gloo Advanced Extauth API is an enterprise-only feature, please upgrade or use the Envoy Extauth API instead"
)

func init() {
	_ = view.Register(extauthConnectedStateView)
}

type TranslatorSyncerExtension struct {
	reports reporter.ResourceReports
}

func NewTranslatorSyncerExtension(_ context.Context, params syncer.TranslatorSyncerExtensionParams) (syncer.TranslatorSyncerExtension, error) {
	return &TranslatorSyncerExtension{reports: params.Reports}, nil
}

func (s *TranslatorSyncerExtension) Sync(ctx context.Context, snap *gloov1.ApiSnapshot, xdsCache envoycache.SnapshotCache) (string, error) {
	ctx = contextutils.WithLogger(ctx, "extAuthTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)

	for _, proxy := range snap.Proxies {
		for _, listener := range proxy.Listeners {
			httpListener, ok := listener.ListenerType.(*gloov1.Listener_HttpListener)
			if !ok {
				// not an http listener - skip it as currently ext auth is only supported for http
				continue
			}

			virtualHosts := httpListener.HttpListener.VirtualHosts

			for _, virtualHost := range virtualHosts {
				if virtualHost.GetOptions().GetExtauth().GetConfigRef() != nil {
					logger.Error(ErrEnterpriseOnly)
					return ExtAuthServerRole, eris.New(ErrEnterpriseOnly)
				}

				for _, route := range virtualHost.Routes {
					if route.GetOptions().GetExtauth().GetConfigRef() != nil {
						logger.Error(ErrEnterpriseOnly)
						return ExtAuthServerRole, eris.New(ErrEnterpriseOnly)
					}

					for _, weightedDestination := range route.GetRouteAction().GetMulti().GetDestinations() {
						if weightedDestination.GetOptions().GetExtauth().GetConfigRef() != nil {
							logger.Error(ErrEnterpriseOnly)
							return ExtAuthServerRole, eris.New(ErrEnterpriseOnly)
						}
					}
				}

			}
		}
	}

	return ExtAuthServerRole, nil
}

func ExtensionName() string {
	return Name
}

func IsUpgrade() bool {
	return false
}

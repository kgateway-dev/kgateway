package extauth

import (
	"context"

	"github.com/rotisserie/eris"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// Compile-time assertion
var (
	_ syncer.TranslatorSyncerExtension            = new(TranslatorSyncerExtension)
	_ syncer.UpgradeableTranslatorSyncerExtension = new(TranslatorSyncerExtension)
)

const (
	Name              = "extauth"
	ExtAuthServerRole = "extauth"
)

var (
	ErrEnterpriseOnly = eris.New("The Gloo Advanced Extauth API is an enterprise-only feature, please upgrade or use the Envoy Extauth API instead")
)

type TranslatorSyncerExtension struct{}

func (s *TranslatorSyncerExtension) ExtensionName() string {
	return Name
}

func (s *TranslatorSyncerExtension) IsUpgrade() bool {
	return false
}

func NewTranslatorSyncerExtension(
	_ context.Context,
	params syncer.TranslatorSyncerExtensionParams,
) (syncer.TranslatorSyncerExtension, error) {
	return &TranslatorSyncerExtension{}, nil
}

func (s *TranslatorSyncerExtension) Sync(
	ctx context.Context,
	snap *gloov1.ApiSnapshot,
	settings *gloov1.Settings,
	xdsCache envoycache.SnapshotCache,
	reports reporter.ResourceReports,
) (string, error) {
	ctx = contextutils.WithLogger(ctx, "extAuthTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)

	getEnterpriseOnlyErr := func() (string, error) {
		logger.Error(ErrEnterpriseOnly.Error())
		return ExtAuthServerRole, ErrEnterpriseOnly
	}

	if settings.GetNamedExtauth() != nil {
		return getEnterpriseOnlyErr()
	}

	for _, proxy := range snap.Proxies {
		for _, listener := range proxy.GetListeners() {
			httpListener, ok := listener.GetListenerType().(*gloov1.Listener_HttpListener)
			if !ok {
				// not an http listener - skip it as currently ext auth is only supported for http
				continue
			}

			virtualHosts := httpListener.HttpListener.VirtualHosts

			for _, virtualHost := range virtualHosts {
				if virtualHost.GetOptions().GetExtauth().GetConfigRef() != nil {
					return getEnterpriseOnlyErr()
				}

				if virtualHost.GetOptions().GetExtauth().GetCustomAuth().GetName() != "" {
					return getEnterpriseOnlyErr()
				}

				for _, route := range virtualHost.GetRoutes() {
					if route.GetOptions().GetExtauth().GetConfigRef() != nil {
						return getEnterpriseOnlyErr()
					}

					if route.GetOptions().GetExtauth().GetCustomAuth().GetName() != "" {
						return getEnterpriseOnlyErr()
					}

					for _, weightedDestination := range route.GetRouteAction().GetMulti().GetDestinations() {
						if weightedDestination.GetOptions().GetExtauth().GetConfigRef() != nil {
							return getEnterpriseOnlyErr()
						}

						if weightedDestination.GetOptions().GetExtauth().GetCustomAuth().GetName() != "" {
							return getEnterpriseOnlyErr()
						}
					}
				}

			}
		}
	}

	return ExtAuthServerRole, nil
}

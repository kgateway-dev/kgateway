package extauth

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/rotisserie/eris"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// Compile-time assertion
var (
	_ syncer.TranslatorSyncerExtension = new(TranslatorSyncerExtension)
)

const (
	ServerRole = "extauth"
)

var (
	ErrEnterpriseOnly = eris.New("The Gloo Advanced Extauth API is an enterprise-only feature, please upgrade or use the Envoy Extauth API instead")
)

type TranslatorSyncerExtension struct{}

func NewTranslatorSyncerExtension(_ context.Context, _ syncer.TranslatorSyncerExtensionParams) (syncer.TranslatorSyncerExtension, error) {
	return &TranslatorSyncerExtension{}, nil
}

func (s *TranslatorSyncerExtension) ID() string {
	return ServerRole
}

func (s *TranslatorSyncerExtension) Sync(
	ctx context.Context,
	snap *gloov1snap.ApiSnapshot,
	settings *gloov1.Settings,
	_ envoycache.SnapshotCache,
	reports reporter.ResourceReports,
) {
	ctx = contextutils.WithLogger(ctx, "extAuthTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)

	getEnterpriseOnlyErr := func() error {
		logger.Error(ErrEnterpriseOnly.Error())
		return ErrEnterpriseOnly
	}

	if settings.GetNamedExtauth() != nil {
		logger.Error(ErrEnterpriseOnly.Error())
	}

	for _, proxy := range snap.Proxies {
		for _, listener := range proxy.GetListeners() {
			virtualHosts := utils.GetVhostsFromListener(listener)

			for _, virtualHost := range virtualHosts {
				if virtualHost.GetOptions().GetExtauth().GetConfigRef() != nil {
					reports.AddError(proxy, getEnterpriseOnlyErr())
				}

				if virtualHost.GetOptions().GetExtauth().GetCustomAuth().GetName() != "" {
					reports.AddError(proxy, getEnterpriseOnlyErr())
				}

				for _, route := range virtualHost.GetRoutes() {
					if route.GetOptions().GetExtauth().GetConfigRef() != nil {
						reports.AddError(proxy, getEnterpriseOnlyErr())
					}

					if route.GetOptions().GetExtauth().GetCustomAuth().GetName() != "" {
						reports.AddError(proxy, getEnterpriseOnlyErr())
					}

					for _, weightedDestination := range route.GetRouteAction().GetMulti().GetDestinations() {
						if weightedDestination.GetOptions().GetExtauth().GetConfigRef() != nil {
							reports.AddError(proxy, getEnterpriseOnlyErr())
						}

						if weightedDestination.GetOptions().GetExtauth().GetCustomAuth().GetName() != "" {
							reports.AddError(proxy, getEnterpriseOnlyErr())
						}
					}
				}

			}
		}
	}
}

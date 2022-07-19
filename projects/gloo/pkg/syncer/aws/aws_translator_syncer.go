package aws

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
	_ syncer.TranslatorSyncerExtension            = new(TranslatorSyncerExtension)
	_ syncer.UpgradeableTranslatorSyncerExtension = new(TranslatorSyncerExtension)
)

const (
	Name       = "aws"
	ServerRole = "aws"
)

var (
	ErrEnterpriseOnly = eris.New("The Gloo Advanced AWS API is an enterprise-only feature, please upgrade to use this functionality")
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
	snap *gloov1snap.ApiSnapshot,
	settings *gloov1.Settings,
	xdsCache envoycache.SnapshotCache,
	reports reporter.ResourceReports,
) (string, error) {
	ctx = contextutils.WithLogger(ctx, "awsTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)

	getEnterpriseOnlyErr := func() (string, error) {
		logger.Error(ErrEnterpriseOnly.Error())
		return ServerRole, ErrEnterpriseOnly
	}

	for _, proxy := range snap.Proxies {
		for _, listener := range proxy.GetListeners() {
			virtualHosts := utils.GetVhostsFromListener(listener)

			for _, virtualHost := range virtualHosts {
				for _, route := range virtualHost.GetRoutes() {
					if awsDestinationSpec := route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws(); awsDestinationSpec != nil {
						if awsDestinationSpec.GetUnwrapAsApiGateway() {
							return getEnterpriseOnlyErr()
						}
					}

					for _, weightedDestination := range route.GetRouteAction().GetMulti().GetDestinations() {
						if awsDestinationSpec := weightedDestination.GetDestination().GetDestinationSpec().GetAws(); awsDestinationSpec != nil {
							if awsDestinationSpec.GetUnwrapAsApiGateway() {
								return getEnterpriseOnlyErr()
							}
						}
					}
				}

			}
		}
	}

	return ServerRole, nil
}

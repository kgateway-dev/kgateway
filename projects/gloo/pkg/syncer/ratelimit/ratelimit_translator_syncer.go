package ratelimit

import (
	"context"
	"github.com/rotisserie/eris"

	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// TODO(marco): generate these in solo-kit
//go:generate mockgen -package mocks -destination mocks/cache.go github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache SnapshotCache
//go:generate mockgen -package mocks -destination mocks/reporter.go github.com/solo-io/solo-kit/pkg/api/v2/reporter Reporter

var (
	rlConnectedStateDescription = "zero indicates gloo detected an error with the rate limit config and did not update its XDS snapshot, check the gloo logs for errors"
	rlConnectedState            = stats.Int64("glooe.ratelimit/connected_state", rlConnectedStateDescription, "1")

	rlConnectedStateView = &view.View{
		Name:        "glooe.ratelimit/connected_state",
		Measure:     rlConnectedState,
		Description: rlConnectedStateDescription,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{},
	}
)

const (
	Name                = "rate-limit"
	RateLimitServerRole = "ratelimit"
	ErrEnterpriseOnly   = "The Gloo Advanced Rate limit API is an enterprise-only feature, please upgrade or use the Envoy rate-limit API instead"
)

func init() {
	_ = view.Register(rlConnectedStateView)
}

type TranslatorSyncerExtension struct {
	reports reporter.ResourceReports
}

func NewTranslatorSyncerExtension(_ context.Context, params syncer.TranslatorSyncerExtensionParams) (syncer.TranslatorSyncerExtension, error) {
	return &TranslatorSyncerExtension{reports: params.Reports}, nil
}

func (s *TranslatorSyncerExtension) Sync(ctx context.Context, snap *gloov1.ApiSnapshot, xdsCache envoycache.SnapshotCache) (string, error) {
	ctx = contextutils.WithLogger(ctx, "rateLimitTranslatorSyncer")
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

				if virtualHost.GetOptions().GetRateLimitConfigs() != nil {
					logger.Warnf(ErrEnterpriseOnly)

					reports := s.reports
					reports.AddError(proxy, eris.New(ErrEnterpriseOnly))

					return RateLimitServerRole, eris.New(ErrEnterpriseOnly)
				}

				if virtualHost.GetOptions().GetRatelimitBasic() != nil {
					logger.Warnf(ErrEnterpriseOnly)

					reports := s.reports
					reports.AddError(proxy, eris.New(ErrEnterpriseOnly))

					return RateLimitServerRole, eris.New(ErrEnterpriseOnly)
				}

				for _, route := range virtualHost.Routes {
					if route.GetOptions().GetRateLimitConfigs() != nil {
						logger.Warnf(ErrEnterpriseOnly)

						reports := s.reports
						reports.AddError(proxy, eris.New(ErrEnterpriseOnly))

						return RateLimitServerRole, eris.New(ErrEnterpriseOnly)
					}

					if route.GetOptions().GetRatelimitBasic() != nil {
						logger.Warnf(ErrEnterpriseOnly)

						reports := s.reports
						reports.AddError(proxy, eris.New(ErrEnterpriseOnly))

						return RateLimitServerRole, eris.New(ErrEnterpriseOnly)
					}

				}

			}
		}
	}

	return RateLimitServerRole, nil
}

func ExtensionName() string {
	return Name
}

func IsUpgrade() bool {
	return false
}

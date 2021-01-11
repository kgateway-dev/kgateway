package ratelimit

import (
	"context"
	"fmt"
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

				// RateLimitConfigs is an enterprise feature https://docs.solo.io/gloo-edge/latest/guides/security/rate_limiting/crds/
				if virtualHost.GetOptions().GetRateLimitConfigs() != nil {
					errorMsg := createErrorMsg("RateLimitConfig")
					logger.Errorf(errorMsg)
					return RateLimitServerRole, eris.New(errorMsg)
				}

				// ratelimitBasic is an enterprise feature https://docs.solo.io/gloo-edge/latest/guides/security/rate_limiting/simple/
				if virtualHost.GetOptions().GetRatelimitBasic() != nil {
					errorMsg := createErrorMsg("ratelimitBasic")
					logger.Errorf(errorMsg)
					return RateLimitServerRole, eris.New(errorMsg)
				}

				// check setActions on vhost
				rlactionsVhost := virtualHost.GetOptions().GetRatelimit().GetRateLimits()
				for _, rlaction := range rlactionsVhost {
					if rlaction.GetSetActions() != nil {
						errorMsg := createErrorMsg("setActions")
						logger.Errorf(errorMsg)
						return RateLimitServerRole, eris.New(errorMsg)
					}
				}

				for _, route := range virtualHost.Routes {
					if route.GetOptions().GetRateLimitConfigs() != nil {
						errorMsg := createErrorMsg("RateLimitConfig")
						logger.Errorf(errorMsg)
						return RateLimitServerRole, eris.New(errorMsg)
					}

					if route.GetOptions().GetRatelimitBasic() != nil {
						errorMsg := createErrorMsg("ratelimitBasic")
						logger.Errorf(errorMsg)
						return RateLimitServerRole, eris.New(errorMsg)
					}

					// check setActions on route
					rlactionsRoute := route.GetOptions().GetRatelimit().GetRateLimits()
					for _, rlaction := range rlactionsRoute {
						if rlaction.GetSetActions() != nil {
							errorMsg := createErrorMsg("setActions")
							logger.Errorf(errorMsg)
							return RateLimitServerRole, eris.New(errorMsg)
						}
					}

				}

			}
		}
	}

	return RateLimitServerRole, nil
}

func createErrorMsg(feature string) string {
	return fmt.Sprintf("The Gloo Advanced Rate limit API '%s' resource is an enterprise-only feature, please upgrade or use the Envoy rate-limit API instead", feature)
}

func ExtensionName() string {
	return Name
}

func IsUpgrade() bool {
	return false
}

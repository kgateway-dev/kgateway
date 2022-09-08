package ratelimit

import (
	"context"
	"fmt"

	"github.com/rotisserie/eris"

	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/go-utils/contextutils"
)

// Compile-time assertion
var (
	_ syncer.TranslatorSyncerExtension = new(translatorSyncerExtension)
)

const (
	ServerRole = "ratelimit"
)

// translatorSyncerExtension is the Open Source variant of the Enterprise translatorSyncerExtension for RateLimit
// TODO (sam-heilbron)
// 	This placeholder is solely used to detect Enterprise features being used in an Open Source installation
//	Once https://github.com/solo-io/gloo/issues/6495 is implemented, we should be able to remove this placeholder altogether
type translatorSyncerExtension struct{}

func NewTranslatorSyncerExtension(_ context.Context, _ syncer.TranslatorSyncerExtensionParams) syncer.TranslatorSyncerExtension {
	return &translatorSyncerExtension{}
}

func (s *translatorSyncerExtension) ID() string {
	return ServerRole
}

func (s *translatorSyncerExtension) Sync(
	ctx context.Context,
	snap *gloov1snap.ApiSnapshot,
	_ *gloov1.Settings,
	_ syncer.SnapshotSetter,
	reports reporter.ResourceReports,
) {
	ctx = contextutils.WithLogger(ctx, "rateLimitTranslatorSyncer")
	logger := contextutils.LoggerFrom(ctx)

	enterpriseOnlyError := func(enterpriseFeature string) error {
		errorMsg := createErrorMsg(enterpriseFeature)
		logger.Errorf(errorMsg)
		return eris.New(errorMsg)
	}

	reports.Accept(snap.Proxies.AsInputResources()...)

	for _, vs := range snap.VirtualServices {
		virtualHost := vs.GetVirtualHost()

		// RateLimitConfigs is an enterprise feature https://docs.solo.io/gloo-edge/latest/guides/security/rate_limiting/crds/
		if virtualHost.GetOptions().GetRateLimitConfigs() != nil {
			reports.AddError(vs, enterpriseOnlyError("RateLimitConfig"))
		}

		// ratelimitBasic is an enterprise feature https://docs.solo.io/gloo-edge/latest/guides/security/rate_limiting/simple/
		if virtualHost.GetOptions().GetRatelimitBasic() != nil {
			reports.AddError(vs, enterpriseOnlyError("ratelimitBasic"))
		}

		// check setActions on vhost
		rlactionsVhost := virtualHost.GetOptions().GetRatelimit().GetRateLimits()
		for _, rlaction := range rlactionsVhost {
			if rlaction.GetSetActions() != nil {
				reports.AddError(vs, enterpriseOnlyError("setActions"))
			}
		}

		// Staged RateLimiting is an enterprise feature
		if virtualHost.GetOptions().GetRateLimitEarlyConfigType() != nil {
			reports.AddError(vs, enterpriseOnlyError("RateLimitEarly"))
		}
		if virtualHost.GetOptions().GetRateLimitRegularConfigType() != nil {
			reports.AddError(vs, enterpriseOnlyError("RateLimitRegular"))
		}

		for _, route := range virtualHost.GetRoutes() {
			if route.GetOptions().GetRateLimitConfigs() != nil {
				reports.AddError(vs, enterpriseOnlyError("RateLimitConfig"))
			}

			if route.GetOptions().GetRatelimitBasic() != nil {
				reports.AddError(vs, enterpriseOnlyError("ratelimitBasic"))
			}

			// check setActions on route
			rlactionsRoute := route.GetOptions().GetRatelimit().GetRateLimits()
			for _, rlaction := range rlactionsRoute {
				if rlaction.GetSetActions() != nil {
					reports.AddError(vs, enterpriseOnlyError("setActions"))
				}
			}

			// Staged RateLimiting is an enterprise feature
			if route.GetOptions().GetRateLimitEarlyConfigType() != nil {
				reports.AddError(vs, enterpriseOnlyError("RateLimitEarly"))
			}
			if route.GetOptions().GetRateLimitRegularConfigType() != nil {
				reports.AddError(vs, enterpriseOnlyError("RateLimitRegular"))
			}
		}
	}
}

func createErrorMsg(feature string) string {
	return fmt.Sprintf("The Gloo Advanced Rate limit API feature '%s' is enterprise-only, please upgrade or use the Envoy rate-limit API instead", feature)
}

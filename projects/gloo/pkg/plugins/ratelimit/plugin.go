package ratelimit

import (
	"fmt"
	"time"

	envoy_extensions_filters_http_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"

	"github.com/rotisserie/eris"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.HttpFilterPlugin  = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
	_ plugins.RoutePlugin       = new(plugin)
)

const (
	ExtensionName = "rate_limit"
	CustomDomain  = "custom"
	RequestType   = "both"

	CustomStage           = uint32(1)
	CustomStageBeforeAuth = uint32(3)
)

var (
	// rate limiting should happen after auth
	DefaultFilterStage = plugins.DuringStage(plugins.RateLimitStage)

	// we may want to rate limit before executing the AuthN and AuthZ stages
	// notably, AuthZ still needs to occur after AuthN
	BeforeAuthStage = plugins.BeforeStage(plugins.AuthNStage)

	DefaultTimeout = prototime.DurationToProto(100 * time.Millisecond)

	ServerNotFound = func(usRef *core.ResourceRef) error {
		return eris.Errorf("ratelimit server upstream not found %s", usRef.String())
	}
)

type plugin struct {
	rlServerSettings *ratelimit.Settings
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	if rlServer := params.Settings.GetRatelimitServer(); rlServer != nil {
		p.rlServerSettings = rlServer
	}
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	if newRateLimits := in.GetOptions().GetRatelimit().GetRateLimits(); len(newRateLimits) > 0 {
		serverSettings := p.getServerSettingsForListener(params.HttpListener)
		rateLimitStage := GetRateLimitStageForServerSettings(serverSettings)
		var err error
		out.RateLimits, err = ToEnvoyRateLimits(params.Ctx, newRateLimits, rateLimitStage)
		return err
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if rateLimits := in.GetOptions().GetRatelimit(); rateLimits != nil {
		if ra := out.GetRoute(); ra != nil {
			serverSettings := p.getServerSettingsForListener(params.HttpListener)
			rateLimitStage := GetRateLimitStageForServerSettings(serverSettings)
			var err error
			ra.RateLimits, err = ToEnvoyRateLimits(params.Ctx, rateLimits.GetRateLimits(), rateLimitStage)
			ra.IncludeVhRateLimits = &wrappers.BoolValue{Value: rateLimits.GetIncludeVhRateLimits()}
			return err
		} else {
			// TODO(yuval-k): maybe return nil here instead and just log a warning?
			return fmt.Errorf("cannot apply rate limits without a route action")
		}
	}
	return nil
}

func (p *plugin) getServerSettingsForListener(listener *v1.HttpListener) *ratelimit.Settings {
	if rlServer := listener.GetOptions().GetRatelimitServer(); rlServer != nil {
		return rlServer
	}

	return p.rlServerSettings
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	serverSettings := p.getServerSettingsForListener(listener)

	upstreamRef := serverSettings.GetRatelimitServerRef()
	if upstreamRef == nil {
		return nil, nil
	}

	// Make sure the server exists
	_, err := params.Snapshot.Upstreams.Find(upstreamRef.GetNamespace(), upstreamRef.GetName())
	if err != nil {
		return nil, ServerNotFound(upstreamRef)
	}

	rateLimitFilter := GenerateEnvoyHttpFilterConfig(serverSettings)
	rateLimitFilterStage := GetFilterStageForRateLimitStage(rateLimitFilter.GetStage())

	stagedRateLimitFilter, err := plugins.NewStagedFilter(
		wellknown.HTTPRateLimit,
		rateLimitFilter,
		rateLimitFilterStage,
	)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{
		stagedRateLimitFilter,
	}, nil
}

func GenerateEnvoyHttpFilterConfig(serverSettings *ratelimit.Settings) *envoy_extensions_filters_http_ratelimit_v3.RateLimit {
	rateLimitStage := GetRateLimitStageForServerSettings(serverSettings)

	return GenerateEnvoyConfigForFilterWith(
		serverSettings.GetRatelimitServerRef(),
		CustomDomain,
		rateLimitStage,
		serverSettings.GetRequestTimeout(),
		serverSettings.GetDenyOnFail(),
		serverSettings.GetEnableXRatelimitHeaders())
}

func GetRateLimitStageForServerSettings(serverSettings *ratelimit.Settings) uint32 {
	rateLimitStage := CustomStage
	if serverSettings.GetRateLimitBeforeAuth() {
		rateLimitStage = CustomStageBeforeAuth
	}
	return rateLimitStage
}

func GetFilterStageForRateLimitStage(rateLimitStage uint32) plugins.FilterStage {
	if rateLimitStage == CustomStageBeforeAuth {
		return BeforeAuthStage
	}
	return DefaultFilterStage
}

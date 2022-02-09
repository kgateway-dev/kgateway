package ratelimit

import (
	"fmt"
	"time"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/duration"
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
	ExtensionName      = "rate_limit"
	EnvoyExtensionName = "envoy-rate-limit"
	CustomDomain       = "custom"
	requestType        = "both"

	CustomStage = 1
)

var (
	// rate limiting should happen after auth
	defaultFilterStage = plugins.DuringStage(plugins.RateLimitStage)

	// we may want to rate limit before executing the AuthN and AuthZ stages
	// notably, AuthZ still needs to occur after AuthN
	beforeAuthStage = plugins.BeforeStage(plugins.AuthNStage)

	DefaultTimeout = prototime.DurationToProto(100 * time.Millisecond)
)

type plugin struct {
	upstreamRef         *core.ResourceRef
	timeout             *duration.Duration
	denyOnFail          bool
	rateLimitBeforeAuth bool
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) error {
	if rlServer := params.Settings.GetRatelimitServer(); rlServer != nil {
		p.upstreamRef = rlServer.GetRatelimitServerRef()
		p.timeout = rlServer.GetRequestTimeout()
		p.denyOnFail = rlServer.GetDenyOnFail()
		p.rateLimitBeforeAuth = rlServer.GetRateLimitBeforeAuth()
	}

	return nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost,
) error {
	if newRateLimits := in.GetOptions().GetRatelimit().GetRateLimits(); len(newRateLimits) > 0 {
		out.RateLimits = toEnvoyRateLimits(params.Ctx, newRateLimits)
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if rateLimits := in.GetOptions().GetRatelimit(); rateLimits != nil {
		if ra := out.GetRoute(); ra != nil {
			ra.RateLimits = toEnvoyRateLimits(params.Ctx, rateLimits.GetRateLimits())
			ra.IncludeVhRateLimits = &wrappers.BoolValue{Value: rateLimits.GetIncludeVhRateLimits()}
		} else {
			// TODO(yuval-k): maybe return nil here instead and just log a warning?
			return fmt.Errorf("cannot apply rate limits without a route action")
		}
	}
	return nil
}

func (p *plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	var upstreamRef *core.ResourceRef
	var timeout *duration.Duration
	var denyOnFail bool
	var rateLimitBeforeAuth bool

	if rlServer := listener.GetOptions().GetRatelimitServer(); rlServer != nil {
		upstreamRef = rlServer.GetRatelimitServerRef()
		timeout = rlServer.GetRequestTimeout()
		denyOnFail = rlServer.GetDenyOnFail()
		rateLimitBeforeAuth = rlServer.GetRateLimitBeforeAuth()
	} else {
		upstreamRef = p.upstreamRef
		timeout = p.timeout
		denyOnFail = p.denyOnFail
		rateLimitBeforeAuth = p.rateLimitBeforeAuth
	}

	if upstreamRef == nil {
		return nil, nil
	}

	customConf := GenerateEnvoyConfigForFilterWith(upstreamRef, CustomDomain, CustomStage, timeout, denyOnFail)

	customStagedFilter, err := plugins.NewStagedFilterWithConfig(
		wellknown.HTTPRateLimit,
		customConf,
		DetermineFilterStage(rateLimitBeforeAuth),
	)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{
		customStagedFilter,
	}, nil
}

// figure out what stage the rate limit plugin should run in given some configuration
func DetermineFilterStage(rateLimitBeforeAuth bool) plugins.FilterStage {
	stage := defaultFilterStage
	if rateLimitBeforeAuth {
		stage = beforeAuthStage
	}

	return stage
}

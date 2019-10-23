package ratelimit

import (
	"fmt"
	"time"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	ExtensionName      = "rate-limit"
	EnvoyExtensionName = "envoy-rate-limit"
	CustomDomain       = "custom"
	requestType        = "both"

	customStage    = 1
	DefaultTimeout = 100 * time.Millisecond

	FilterName = "envoy.rate_limit"
)

var (
	// rate limiting should happen after auth
	defaultFilterStage = plugins.DuringStage(plugins.RateLimitStage)

	// we may want to rate limit before executing the AuthN and AuthZ stages
	// notably, AuthZ still needs to occur after AuthN
	beforeAuthStage = plugins.BeforeStage(plugins.AuthNStage)
)

type Plugin struct {
	upstreamRef         *core.ResourceRef
	timeout             *time.Duration
	denyOnFail          bool
	rateLimitBeforeAuth bool
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

//TODO(kdorosh) delete once support for old config is dropped
func (p *Plugin) handleDeprecatedPluginConfig(params plugins.InitParams) error {
	var settings ratelimit.Settings
	p.upstreamRef = nil
	err := utils.ExtensionsToProto(params.ExtensionsSettings, ExtensionName, &settings)
	if err != nil {
		if err == utils.NotFoundError {
			return nil
		}
		return err
	}

	p.upstreamRef = settings.RatelimitServerRef
	p.timeout = settings.RequestTimeout
	p.denyOnFail = settings.DenyOnFail
	return nil
}

func (p *Plugin) Init(params plugins.InitParams) error {
	if err := p.handleDeprecatedPluginConfig(params); err != nil {
		return err
	}

	if rlServer := params.Settings.GetRatelimitServer(); rlServer != nil {
		p.upstreamRef = rlServer.RatelimitServerRef
		p.timeout = rlServer.RequestTimeout
		p.denyOnFail = rlServer.DenyOnFail
		p.rateLimitBeforeAuth = rlServer.RateLimitBeforeAuth
	}

	return nil
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	rateLimit, err := p.handleDeprecatedVirtualHostCustom(params, in)
	if err != nil {
		return err
	}

	if rl := in.GetVirtualHostPlugins().GetRatelimit(); rl != nil {
		rateLimit = rl
	}

	if rateLimit == nil {
		// no rate limit virtual host config found, nothing to do here
		return nil
	}

	out.RateLimits = generateCustomEnvoyConfigForVhost(rateLimit.RateLimits)

	return nil
}

//TODO(kdorosh) delete once support for old config is dropped
func (p *Plugin) handleDeprecatedVirtualHostCustom(params plugins.VirtualHostParams, in *v1.VirtualHost) (*ratelimit.RateLimitVhostExtension, error) {
	var rateLimit ratelimit.RateLimitVhostExtension
	err := utils.UnmarshalExtension(in.VirtualHostPlugins, EnvoyExtensionName, &rateLimit)
	if err != nil {
		if err == utils.NotFoundError {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "Error converting proto to vhost rate limit plugin")
	}
	return &rateLimit, nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	rateLimit, err := p.handleDeprecatedProcessRoute(params, in)
	if err != nil {
		return err
	}

	if rl := in.GetRoutePlugins().GetRatelimit(); rl != nil {
		rateLimit = rl
	}

	if rateLimit == nil {
		// no rate limit route config found, nothing to do here
		return nil
	}

	ra := out.GetRoute()
	if ra != nil {
		ra.RateLimits = generateCustomEnvoyConfigForVhost(rateLimit.RateLimits)
		ra.IncludeVhRateLimits = &types.BoolValue{Value: rateLimit.IncludeVhRateLimits}
	} else {
		// TODO(yuval-k): maybe return nil here instead and just log a warning?
		return fmt.Errorf("cannot apply rate limits without a route action")
	}

	return nil
}

//TODO(kdorosh) delete once support for old config is dropped
func (p *Plugin) handleDeprecatedProcessRoute(params plugins.RouteParams, in *v1.Route) (*ratelimit.RateLimitRouteExtension, error) {
	var rateLimit ratelimit.RateLimitRouteExtension
	err := utils.UnmarshalExtension(in.RoutePlugins, EnvoyExtensionName, &rateLimit)
	if err != nil {
		if err == utils.NotFoundError {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "Error converting proto any to vhost rate limit plugin")
	}
	return &rateLimit, nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if p.upstreamRef == nil {
		return nil, nil
	}

	customConf := generateEnvoyConfigForCustomFilter(*p.upstreamRef, p.timeout, p.denyOnFail)

	filterStage := defaultFilterStage
	if p.rateLimitBeforeAuth {
		filterStage = beforeAuthStage
	}

	customStagedFilter, err := plugins.NewStagedFilterWithConfig(FilterName, customConf, filterStage)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{
		customStagedFilter,
	}, nil
}

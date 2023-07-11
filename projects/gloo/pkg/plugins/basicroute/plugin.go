package basicroute

import (
	"context"
	"fmt"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	_ plugins.RoutePlugin       = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
)

const (
	ExtensionName = "basic_route"
)

// Handles a RoutePlugin APIs which map directly to basic Envoy config
type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	if in.GetOptions() == nil {
		return nil
	}
	return applyRetriesVhost(in, out)
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	// This plugin is only available for routeActions. return early if a different action is specified.
	if _, ok := in.GetAction().(*v1.Route_RouteAction); !ok {
		return nil
	}

	if in.GetOptions() == nil {
		return nil
	}

	if err := applyPrefixRewrite(in, out); err != nil {
		return err
	}
	if err := applyRegexRewrite(params, in, out); err != nil {
		return err
	}
	if err := applyTimeout(in, out); err != nil {
		return err
	}
	if err := applyIdleTimeout(in, out); err != nil {
		return err
	}
	if err := applyMaxStreamDuration(in, out); err != nil {
		return err
	}
	if err := applyRetries(in, out); err != nil {
		return err
	}
	if err := applyHostRewrite(params.Ctx, in, out); err != nil {
		return err
	}
	if err := applyUpgrades(in, out); err != nil {
		return err
	}

	return nil
}

func applyPrefixRewrite(in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.GetOptions().GetPrefixRewrite() == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("prefix rewrite is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.GetAction())
	}
	routeAction.Route.PrefixRewrite = in.GetOptions().GetPrefixRewrite().GetValue()
	return nil
}

func applyRegexRewrite(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.GetOptions().GetRegexRewrite() == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("regex rewrite is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a regex, but output Envoy object "+
			"had nil route", in.GetAction())
	}
	regexRewrite, err := regexutils.ConvertRegexMatchAndSubstitute(params.Ctx, in.GetOptions().GetRegexRewrite())
	if err != nil {
		return err
	}
	routeAction.Route.RegexRewrite = regexRewrite
	return nil
}

func applyTimeout(in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.GetOptions().GetTimeout() == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("timeout is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.GetAction())
	}

	routeAction.Route.Timeout = in.GetOptions().GetTimeout()
	return nil
}

func applyIdleTimeout(in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.GetOptions().GetIdleTimeout() == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("timeout is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.GetAction())
	}

	routeAction.Route.IdleTimeout = in.GetOptions().GetIdleTimeout()
	return nil
}

func applyMaxStreamDuration(in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.GetOptions().GetMaxStreamDuration() == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("Max Stream Duration is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a max stream duration, but output Envoy object "+
			"had nil route", in.GetAction())
	}
	inMaxStreamDuration := in.GetOptions().GetMaxStreamDuration()
	routeAction.Route.MaxStreamDuration = &envoy_config_route_v3.RouteAction_MaxStreamDuration{
		MaxStreamDuration:       inMaxStreamDuration.GetMaxStreamDuration(),
		GrpcTimeoutHeaderMax:    inMaxStreamDuration.GetGrpcTimeoutHeaderMax(),
		GrpcTimeoutHeaderOffset: inMaxStreamDuration.GetGrpcTimeoutHeaderOffset(),
	}
	return nil
}

func applyRetries(in *v1.Route, out *envoy_config_route_v3.Route) error {
	policy := in.GetOptions().GetRetries()
	if policy == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("retries is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.GetAction())
	}

	var err error
	routeAction.Route.RetryPolicy, err = convertPolicy(policy)
	if err != nil {
		return err
	}
	return nil
}

// Put functions we want to mock in tests in here
var (
	ConvertRegexMatchAndSubstitute = regexutils.ConvertRegexMatchAndSubstitute
)

func applyHostRewrite(ctx context.Context, in *v1.Route, out *envoy_config_route_v3.Route) error {
	hostRewriteType := in.GetOptions().GetHostRewriteType()
	if hostRewriteType == nil {
		return nil
	}
	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("hostRewrite is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.GetAction())
	}
	switch rewriteType := hostRewriteType.(type) {
	case *v1.RouteOptions_HostRewrite:
		routeAction.Route.HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_HostRewriteLiteral{
			HostRewriteLiteral: rewriteType.HostRewrite,
		}
	case *v1.RouteOptions_AutoHostRewrite:
		routeAction.Route.HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_AutoHostRewrite{
			AutoHostRewrite: rewriteType.AutoHostRewrite,
		}

	case *v1.RouteOptions_HostRewritePathRegex:
		regex, err := ConvertRegexMatchAndSubstitute(ctx, rewriteType.HostRewritePathRegex)
		if err != nil {
			return err
		}

		routeAction.Route.HostRewriteSpecifier = &envoy_config_route_v3.RouteAction_HostRewritePathRegex{
			HostRewritePathRegex: regex,
		}

	default:
		return errors.Errorf("unimplemented host rewrite type: %T", rewriteType)
	}
	if in.GetOptions().GetAppendXForwardedHost() != nil {
		routeAction.Route.AppendXForwardedHost = in.GetOptions().GetAppendXForwardedHost().GetValue()
	}
	return nil
}

func applyUpgrades(in *v1.Route, out *envoy_config_route_v3.Route) error {
	upgrades := in.GetOptions().GetUpgrades()
	if upgrades == nil {
		return nil
	}

	routeAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("upgrades are only available for Route Actions")
	}

	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.GetAction())
	}

	routeAction.Route.UpgradeConfigs = make([]*envoy_config_route_v3.RouteAction_UpgradeConfig, len(upgrades))

	for i, config := range upgrades {
		switch upgradeType := config.GetUpgradeType().(type) {
		case *protocol_upgrade.ProtocolUpgradeConfig_Websocket:
			routeAction.Route.GetUpgradeConfigs()[i] = &envoy_config_route_v3.RouteAction_UpgradeConfig{
				UpgradeType: upgradeconfig.WebSocketUpgradeType,
				Enabled:     config.GetWebsocket().GetEnabled(),
			}
		case *protocol_upgrade.ProtocolUpgradeConfig_Connect:
			routeAction.Route.GetUpgradeConfigs()[i] = &envoy_config_route_v3.RouteAction_UpgradeConfig{
				UpgradeType: upgradeconfig.ConnectUpgradeType,
				Enabled:     config.GetConnect().GetEnabled(),
			}
		default:
			return errors.Errorf("unimplemented upgrade type: %T", upgradeType)
		}
	}

	return upgradeconfig.ValidateRouteUpgradeConfigs(routeAction.Route.GetUpgradeConfigs())
}

func applyRetriesVhost(in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error {
	var err error
	out.RetryPolicy, err = convertPolicy(in.GetOptions().GetRetries())
	if err != nil {
		return err
	}
	return nil
}

func convertPolicy(policy *retries.RetryPolicy) (*envoy_config_route_v3.RetryPolicy, error) {
	if policy == nil {
		return nil, nil
	}

	numRetries := policy.GetNumRetries()
	if numRetries == 0 {
		numRetries = 1
	}

	v3RetryPolicyBackOff := &envoy_config_route_v3.RetryPolicy_RetryBackOff{}

	// Let's make some checks
	if retryPolicyInterval := policy.GetRetryBackOff(); retryPolicyInterval != nil {

		baseInterval := retryPolicyInterval.GetBaseInterval()
		maxInterval := retryPolicyInterval.GetMaxInterval()

		// Is the max interval larger than or equal to the base interval?
		if baseInterval != nil && maxInterval != nil {
			if baseInterval.AsDuration().Milliseconds() > maxInterval.AsDuration().Milliseconds() {
				return nil,
					fmt.Errorf("base interval: %d is > max interval: %d",
						baseInterval.AsDuration().Milliseconds(),
						maxInterval.AsDuration().Milliseconds())
			}
		}

		// Check if the max interval is defined without the base interval
		if maxInterval != nil && baseInterval == nil {
			return nil, fmt.Errorf("max interval was defined, but the base interval was not")
		}

		// Check if the base interval is defined
		if baseInterval != nil {

			// If the base interval is defined, check that it's greater than zero milliseconds
			if dur := baseInterval.AsDuration().Milliseconds(); dur <= 0 {
				return nil,
					errors.Errorf("base interval for retry backoff was <= than 0 | you provided: %d", dur)
			} else {
				v3RetryPolicyBackOff.BaseInterval = baseInterval
			}
		}

		// Check if the max interval is defined
		if maxInterval != nil {

			// If the max interval is defined, check that it's greater than zero
			if dur := maxInterval.AsDuration().Milliseconds(); dur <= 0 {
				return nil,
					errors.Errorf("max interval for retry backoff was <= than 0 | you provided: %d", dur)
			} else {
				v3RetryPolicyBackOff.MaxInterval = maxInterval
			}
		}

		// If max and/or/both base intervals are defined, return a RetryPolicy object that contains them
		return &envoy_config_route_v3.RetryPolicy{
			RetryOn:       policy.GetRetryOn(),
			NumRetries:    &wrappers.UInt32Value{Value: numRetries},
			PerTryTimeout: policy.GetPerTryTimeout(),
			RetryBackOff:  v3RetryPolicyBackOff,
		}, nil
	}

	return &envoy_config_route_v3.RetryPolicy{
		RetryOn:       policy.GetRetryOn(),
		NumRetries:    &wrappers.UInt32Value{Value: numRetries},
		PerTryTimeout: policy.GetPerTryTimeout(),
	}, nil
}

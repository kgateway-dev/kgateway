package basicroute

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	v32 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type Plugin struct{}

var _ plugins.RoutePlugin = NewPlugin()
var _ plugins.VirtualHostPlugin = NewPlugin()

// Handles a RoutePlugin APIs which map directly to basic Envoy config
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	if in.Options == nil {
		return nil
	}
	return applyRetriesVhost(in, out)
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.Options == nil {
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
	if err := applyRetries(in, out); err != nil {
		return err
	}
	if err := applyHostRewrite(in, out); err != nil {
		return err
	}
	if err := applyUpgrades(in, out); err != nil {
		return err
	}

	return nil
}

func applyPrefixRewrite(in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.Options.PrefixRewrite == nil {
		return nil
	}
	routeAction, ok := out.Action.(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("prefix rewrite is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.Action)
	}
	routeAction.Route.PrefixRewrite = in.Options.PrefixRewrite.Value
	return nil
}

func applyRegexRewrite(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.Options.RegexRewrite == nil {
		return nil
	}
	routeAction, ok := out.Action.(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("regex rewrite is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a regex, but output Envoy object "+
			"had nil route", in.Action)
	}
	routeAction.Route.RegexRewrite = convertRegexMatchAndSubstitute(params, in.Options.RegexRewrite)
	return nil
}

func applyTimeout(in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.Options.Timeout == nil {
		return nil
	}
	routeAction, ok := out.Action.(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("timeout is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.Action)
	}

	routeAction.Route.Timeout = in.Options.Timeout
	return nil
}

func applyRetries(in *v1.Route, out *envoy_config_route_v3.Route) error {
	policy := in.Options.Retries
	if policy == nil {
		return nil
	}
	routeAction, ok := out.Action.(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("retries is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.Action)
	}

	routeAction.Route.RetryPolicy = convertPolicy(policy)
	return nil
}

func applyHostRewrite(in *v1.Route, out *envoy_config_route_v3.Route) error {
	hostRewriteType := in.GetOptions().GetHostRewriteType()
	if hostRewriteType == nil {
		return nil
	}
	routeAction, ok := out.Action.(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("hostRewrite is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.Action)
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
	default:
		return errors.Errorf("unimplemented host rewrite type: %T", rewriteType)
	}

	return nil
}

func applyUpgrades(in *v1.Route, out *envoy_config_route_v3.Route) error {
	upgrades := in.GetOptions().GetUpgrades()
	if upgrades == nil {
		return nil
	}

	routeAction, ok := out.Action.(*envoy_config_route_v3.Route_Route)
	if !ok {
		return errors.Errorf("upgrades are only available for Route Actions")
	}

	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.Action)
	}

	routeAction.Route.UpgradeConfigs = make([]*envoy_config_route_v3.RouteAction_UpgradeConfig, len(upgrades))

	for i, config := range upgrades {
		switch upgradeType := config.GetUpgradeType().(type) {
		case *protocol_upgrade.ProtocolUpgradeConfig_Websocket:
			routeAction.Route.UpgradeConfigs[i] = &envoy_config_route_v3.RouteAction_UpgradeConfig{
				UpgradeType: upgradeconfig.WebSocketUpgradeType,
				Enabled:     config.GetWebsocket().GetEnabled(),
			}
		default:
			return errors.Errorf("unimplemented upgrade type: %T", upgradeType)
		}
	}

	return upgradeconfig.ValidateRouteUpgradeConfigs(routeAction.Route.UpgradeConfigs)
}

func applyRetriesVhost(in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error {
	out.RetryPolicy = convertPolicy(in.Options.Retries)
	return nil
}

func convertPolicy(policy *retries.RetryPolicy) *envoy_config_route_v3.RetryPolicy {
	if policy == nil {
		return nil
	}

	numRetries := policy.NumRetries
	if numRetries == 0 {
		numRetries = 1
	}

	return &envoy_config_route_v3.RetryPolicy{
		RetryOn:       policy.GetRetryOn(),
		NumRetries:    &wrappers.UInt32Value{Value: numRetries},
		PerTryTimeout: policy.GetPerTryTimeout(),
	}
}

func convertRegexMatchAndSubstitute(params plugins.RouteParams, in *v32.RegexMatchAndSubstitute) *envoy_type_matcher_v3.RegexMatchAndSubstitute {
	if in == nil {
		return nil
	}

	out := &envoy_type_matcher_v3.RegexMatchAndSubstitute{
		Pattern:      regexutils.NewRegex(params.Ctx, in.Pattern.Regex),
		Substitution: in.Substitution,
	}
	switch inET := in.Pattern.EngineType.(type) {
	case *v32.RegexMatcher_GoogleRe2:
		outET := out.Pattern.EngineType.(*envoy_type_matcher_v3.RegexMatcher_GoogleRe2)
		if inET.GoogleRe2.MaxProgramSize != nil && (outET.GoogleRe2.MaxProgramSize == nil || inET.GoogleRe2.MaxProgramSize.Value < outET.GoogleRe2.MaxProgramSize.Value) {
			out.Pattern = regexutils.NewRegexWithProgramSize(in.Pattern.Regex, &inET.GoogleRe2.MaxProgramSize.Value)
		}
	}

	return out
}

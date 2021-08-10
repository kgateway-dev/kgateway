package stats

import (
	"context"
	"sort"
	"strings"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	regexutils "github.com/solo-io/gloo/pkg/utils/regexutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	invalidVirtualClusterErr = func(err error, vcName string) error {
		return eris.Wrapf(err, "failed to process virtual cluster [%s]", vcName)
	}
	missingNameErr    = eris.Errorf("name is required")
	missingPatternErr = eris.Errorf("pattern is required")
	invalidMethodErr  = func(methodName string) error {
		return eris.Errorf("invalid method name [%s]. Allowed values: %s", methodName, validMethodNames())
	}
)

type Plugin struct{}

// Compile-time assertion
var _ plugins.VirtualHostPlugin = &Plugin{}

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
	if in.GetOptions() == nil || in.GetOptions().GetStats() == nil {
		return nil
	}

	vClusters, err := converter{ctx: params.Ctx}.convertVirtualClusters(params, in.GetOptions().GetStats())
	if err != nil {
		return err
	}
	out.VirtualClusters = vClusters

	return nil
}

type converter struct {
	ctx context.Context
}

func (c converter) convertVirtualClusters(
	params plugins.VirtualHostParams,
	statsConfig *stats.Stats,
) ([]*envoy_config_route_v3.VirtualCluster, error) {
	var result []*envoy_config_route_v3.VirtualCluster
	for _, virtualCluster := range statsConfig.GetVirtualClusters() {

		name, err := c.validateName(virtualCluster.GetName())
		if err != nil {
			return nil, invalidVirtualClusterErr(err, virtualCluster.GetName())
		}

		if virtualCluster.GetPattern() == "" {
			return nil, invalidVirtualClusterErr(missingPatternErr, virtualCluster.GetName())
		}

		method, err := c.validateHttpMethod(virtualCluster.GetMethod())
		if err != nil {
			return nil, invalidVirtualClusterErr(err, virtualCluster.GetName())
		}

		headermatcher := []*envoy_config_route_v3.HeaderMatcher{{
			Name: ":path",
			HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_SafeRegexMatch{
				SafeRegexMatch: regexutils.NewRegex(params.Ctx, virtualCluster.GetPattern()),
			},
		}}

		if method != "" {
			headermatcher = append(headermatcher, &envoy_config_route_v3.HeaderMatcher{
				Name: ":method",
				HeaderMatchSpecifier: &envoy_config_route_v3.HeaderMatcher_ExactMatch{
					ExactMatch: method,
				},
			})
		}

		// method and path must be not empty
		result = append(result, &envoy_config_route_v3.VirtualCluster{
			Name:    name,
			Headers: headermatcher,
		})
	}
	return result, nil
}

func (c converter) validateName(name string) (string, error) {
	if name == "" {
		return "", missingNameErr
	}
	return utils.SanitizeForEnvoy(c.ctx, name, "virtual cluster"), nil
}

func (c converter) validateHttpMethod(methodName string) (string, error) {
	if methodName == "" {
		return "", nil
	}
	key := strings.ToUpper(methodName)
	_, ok := envoy_config_core_v3.RequestMethod_value[key]
	if !ok {
		return "", invalidMethodErr(methodName)
	}
	return key, nil
}

func validMethodNames() string {
	var names []string
	for methodName := range envoy_config_core_v3.RequestMethod_value {
		names = append(names, methodName)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

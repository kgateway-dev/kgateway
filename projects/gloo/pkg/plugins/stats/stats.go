package stats

import (
	"context"
	"sort"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/rotisserie/eris"
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

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	if in.GetOptions() == nil || in.GetOptions().GetStats() == nil {
		return nil
	}

	vClusters, err := converter{ctx: params.Ctx}.convertVirtualClusters(in.GetOptions().GetStats())
	if err != nil {
		return err
	}
	out.VirtualClusters = vClusters

	return nil
}

type converter struct {
	ctx context.Context
}

func (c converter) convertVirtualClusters(statsConfig *stats.Stats) ([]*envoyroute.VirtualCluster, error) {
	var result []*envoyroute.VirtualCluster
	for _, virtualCluster := range statsConfig.VirtualClusters {

		name, err := c.validateName(virtualCluster.Name)
		if err != nil {
			return nil, invalidVirtualClusterErr(err, virtualCluster.Name)
		}

		if virtualCluster.Pattern == "" {
			return nil, invalidVirtualClusterErr(missingPatternErr, virtualCluster.Name)
		}

		method, err := c.validateHttpMethod(virtualCluster.Method)
		if err != nil {
			return nil, invalidVirtualClusterErr(err, virtualCluster.Name)
		}

		result = append(result, &envoyroute.VirtualCluster{
			Name:    name,
			Pattern: virtualCluster.Pattern,
			Method:  method,
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

func (c converter) validateHttpMethod(methodName string) (envoycore.RequestMethod, error) {
	if methodName == "" {
		return envoycore.RequestMethod_METHOD_UNSPECIFIED, nil
	}
	method, ok := envoycore.RequestMethod_value[strings.ToUpper(methodName)]
	if !ok {
		return 0, invalidMethodErr(methodName)
	}
	return envoycore.RequestMethod(method), nil
}

func validMethodNames() string {
	var names []string
	for methodName := range envoycore.RequestMethod_value {
		names = append(names, methodName)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

package healthcheck

import (
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhealthcheck "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	ExtensionName = "health_check"
)

// filter info
var pluginStage = plugins.AfterStage(plugins.AuthZStage)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

	healthCheck := listener.GetOptions().GetHealthCheck()

	if healthCheck == nil {
		return nil, nil
	}

	path := healthCheck.GetPath()

	if path == "" {
		return nil, errors.Errorf("health check path cannot be \"\"")
	}

	hc := &envoyhealthcheck.HealthCheck{
		PassThroughMode: &wrappers.BoolValue{Value: false},
		Headers: []*route.HeaderMatcher{{
			Name: ":path",
			HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
				ExactMatch: path,
			},
		}},
	}

	healthCheckFilter, err := plugins.NewStagedFilter(wellknown.HealthCheck, hc, pluginStage)
	if err != nil {
		return nil, errors.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{healthCheckFilter}, nil
}

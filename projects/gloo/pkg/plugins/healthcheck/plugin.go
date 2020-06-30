package healthcheck

import (
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhealthcheck "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/health_check/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// filter info
var pluginStage = plugins.AfterStage(plugins.AuthZStage)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.HttpFilterPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

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

	healthCheckFilter, err := plugins.NewStagedFilterWithConfig(wellknown.HealthCheck, hc, pluginStage)
	if err != nil {
		return nil, errors.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{healthCheckFilter}, nil
}

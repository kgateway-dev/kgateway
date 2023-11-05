package urlrewrite

import (
	"context"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx context.Context,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	config := filter.RequestRedirect
	if config == nil {
		return errors.Errorf("RequestRedirect filter supplied does not define requestRedirect config")
	}

	if outputRoute.Action != nil {
		return errors.Errorf("RequestRedirect route cannot have destinations")
	}

	if config.StatusCode == nil {
		return errors.Errorf("RequestRedirect: unsupported value")
	}

	return nil
}

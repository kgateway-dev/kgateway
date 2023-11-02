package redirect

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

	outputRoute.Action = &v1.Route_RedirectAction{
		RedirectAction: &v1.RedirectAction{
			// TODO: support extended fields on RedirectAction
			HostRedirect: translateHostname(config.Hostname),
			ResponseCode: translateStatusCode(*config.StatusCode),
		},
	}

	return nil
}

func translateHostname(hostname *gwv1.PreciseHostname) string {
	if hostname == nil {
		return ""
	}
	return string(*hostname)
}

func translateStatusCode(i int) v1.RedirectAction_RedirectResponseCode {
	switch i {
	case 301:
		return v1.RedirectAction_MOVED_PERMANENTLY
	case 302:
		return v1.RedirectAction_FOUND
	case 303:
		return v1.RedirectAction_SEE_OTHER
	case 307:
		return v1.RedirectAction_TEMPORARY_REDIRECT
	case 308:
		return v1.RedirectAction_PERMANENT_REDIRECT
	default:
		return v1.RedirectAction_MOVED_PERMANENTLY
	}
}

package redirect

import (
	"context"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	errors "github.com/rotisserie/eris"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx context.Context,
	filter gwv1.HTTPRouteFilter,
	outputRoute *routev3.Route,
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

	outputRoute.Action = &routev3.Route_Redirect{
		Redirect: &routev3.RedirectAction{
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

func translateStatusCode(i int) routev3.RedirectAction_RedirectResponseCode {
	switch i {
	case 301:
		return routev3.RedirectAction_MOVED_PERMANENTLY
	case 302:
		return routev3.RedirectAction_FOUND
	case 303:
		return routev3.RedirectAction_SEE_OTHER
	case 307:
		return routev3.RedirectAction_TEMPORARY_REDIRECT
	case 308:
		return routev3.RedirectAction_PERMANENT_REDIRECT
	default:
		return routev3.RedirectAction_MOVED_PERMANENTLY
	}
}

package redirect

import (
	"strings"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx *filterplugins.RouteContext,
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

	outputRoute.Action = &v1.Route_RedirectAction{
		RedirectAction: &v1.RedirectAction{
			// TODO: support extended fields on RedirectAction
			HttpsRedirect: config.Scheme != nil && strings.ToLower(*config.Scheme) == "https",
			HostRedirect:  translateHostname(config.Hostname),
			ResponseCode:  translateStatusCode(*config.StatusCode),
			PortRedirect:  translatePort(config.Port),
		},
	}

	translatePathRewrite(config.Path, outputRoute)

	return nil
}

func translatePort(port *gwv1.PortNumber) uint32 {
	if port == nil {
		return 0
	}
	return uint32(*port)
}

func translateHostname(hostname *gwv1.PreciseHostname) string {
	if hostname == nil {
		return ""
	}
	return string(*hostname)
}

func translatePathRewrite(pathRewrite *gwv1.HTTPPathModifier, outputRoute *v1.Route) {
	if pathRewrite == nil {
		return
	}
	switch pathRewrite.Type {
	case gwv1.FullPathHTTPPathModifier:
		outputRoute.GetRedirectAction().PathRewriteSpecifier = &v1.RedirectAction_PathRedirect{
			PathRedirect: *pathRewrite.ReplaceFullPath,
		}
	case gwv1.PrefixMatchHTTPPathModifier:
		outputRoute.GetRedirectAction().PathRewriteSpecifier = &v1.RedirectAction_PrefixRewrite{
			PrefixRewrite: *pathRewrite.ReplacePrefixMatch,
		}
	}
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
		return v1.RedirectAction_FOUND
	}
}

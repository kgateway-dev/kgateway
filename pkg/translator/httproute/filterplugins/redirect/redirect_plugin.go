package redirect

import (
	"strings"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx *filterplugins.RouteContext,
	filter gwv1.HTTPRouteFilter,
	outputRoute *routev3.Route,
) error {
	config := filter.RequestRedirect
	if config == nil {
		return errors.Errorf("RequestRedirect filter supplied does not define requestRedirect config")
	}

	if outputRoute.GetAction() != nil {
		return errors.Errorf("RequestRedirect route cannot have destinations")
	}
	statusCode := 302
	if config.StatusCode != nil {
		statusCode = *config.StatusCode
	}

	redirectAction := &routev3.RedirectAction{
		// TODO: support extended fields on RedirectAction
		HostRedirect: translateHostname(config.Hostname),
		ResponseCode: translateStatusCode(statusCode),
		PortRedirect: translatePort(config.Port),
	}

	if config.Scheme != nil && strings.ToLower(*config.Scheme) == "https" {
		redirectAction.SchemeRewriteSpecifier = &routev3.RedirectAction_HttpsRedirect{HttpsRedirect: true}
	}
	outputRoute.Action = &routev3.Route_Redirect{
		Redirect: redirectAction,
	}

	translatePathRewrite(config.Path, redirectAction)

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

func translatePathRewrite(pathRewrite *gwv1.HTTPPathModifier, redirectAction *routev3.RedirectAction) {
	if pathRewrite == nil {
		return
	}
	replaceFullPath := "/"
	if pathRewrite.ReplaceFullPath != nil {
		replaceFullPath = *pathRewrite.ReplaceFullPath
	}
	prefixRewrite := "/"
	if pathRewrite.ReplacePrefixMatch != nil {
		prefixRewrite = *pathRewrite.ReplacePrefixMatch
	}
	switch pathRewrite.Type {
	case gwv1.FullPathHTTPPathModifier:
		redirectAction.PathRewriteSpecifier = &routev3.RedirectAction_PathRedirect{
			PathRedirect: replaceFullPath,
		}
	case gwv1.PrefixMatchHTTPPathModifier:
		redirectAction.PathRewriteSpecifier = &routev3.RedirectAction_PrefixRewrite{
			PrefixRewrite: prefixRewrite,
		}
	}
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
		return routev3.RedirectAction_FOUND
	}
}

package printers

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// PrintTable prints virtual services using tables to io.Writer
func VirtualServiceTable(list []*v1.VirtualService, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Virtual Service", "Domains", "SSL", "Status", "Plugins", "Routes"})

	for _, v := range list {
		name := v.GetMetadata().Name
		domains := domains(v)
		ssl := sslConfig(v)
		status := v.Status.State.String()
		routes := routeList(v)
		plugins := vhPlugins(v)

		if len(routes) == 0 {
			routes = []string{""}
		}
		for i, line := range routes {
			if i == 0 {
				table.Append([]string{name, domains, ssl, status, plugins, line})
			} else {
				table.Append([]string{"", "", "", "", "", line})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func routeList(v *v1.VirtualService) []string {
	if len(v.VirtualHost.Routes) == 0 {
		return nil
	}
	var routes []string
	for _, route := range v.VirtualHost.Routes {
		routes = append(routes, fmt.Sprintf("%v -> %v", matcherString(route.Matcher), destinationString(route)))
	}
	return routes
}

func vhPlugins(v *v1.VirtualService) string {
	var pluginStr string
	if v.VirtualHost.VirtualHostPlugins != nil {
		// TODO: fill this when there are vhost plugins
	}
	return pluginStr
}

func matcherString(matcher *gloov1.Matcher) string {
	switch ps := matcher.PathSpecifier.(type) {
	case *gloov1.Matcher_Exact:
		return ps.Exact
	case *gloov1.Matcher_Prefix:
		return ps.Prefix
	case *gloov1.Matcher_Regex:
		return ps.Regex
	}
	return ""
}

func destinationString(route *gloov1.Route) string {
	switch action := route.Action.(type) {
	case *gloov1.Route_RouteAction:
		switch dest := action.RouteAction.Destination.(type) {
		case *gloov1.RouteAction_Multi:
			return fmt.Sprintf("%v destinations", len(dest.Multi.Destinations))
		case *gloov1.RouteAction_Single:
			return dest.Single.Upstream.Name
		}
	case *gloov1.Route_DirectResponseAction:
		return strconv.Itoa(int(action.DirectResponseAction.Status))
	case *gloov1.Route_RedirectAction:
		return action.RedirectAction.HostRedirect
	}
	return ""
}

func domains(v *v1.VirtualService) string {
	if v.VirtualHost.Domains == nil || len(v.VirtualHost.Domains) == 0 {
		return ""
	}

	return strings.Join(v.VirtualHost.Domains, ", ")
}

func sslConfig(v *v1.VirtualService) string {
	if v.GetSslConfig() == nil {
		return "none"
	} else {
		switch v.GetSslConfig().SslSecrets.(type) {
		case *gloov1.SslConfig_SecretRef:
			return "secret_ref"
		case *gloov1.SslConfig_SslFiles:
			return "ssl_files"
		default:
			return "unknown"
		}
	}
}

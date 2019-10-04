package printers

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func PrintVirtualServices(virtualServices v1.VirtualServiceList, outputType OutputType) error {
	if outputType == KUBE_YAML {
		return PrintKubeCrdList(virtualServices.AsInputResources(), v1.VirtualServiceCrd)
	}
	return cliutils.PrintList(outputType.String(), "", virtualServices,
		func(data interface{}, w io.Writer) error {
			VirtualServiceTable(data.(v1.VirtualServiceList), w)
			return nil
		}, os.Stdout)
}

func PrintRouteTables(routeTables v1.RouteTableList, outputType OutputType) error {
	if outputType == KUBE_YAML {
		return PrintKubeCrdList(routeTables.AsInputResources(), v1.RouteTableCrd)
	}
	return cliutils.PrintList(outputType.String(), "", routeTables,
		func(data interface{}, w io.Writer) error {
			RouteTableTable(data.(v1.RouteTableList), w)
			return nil
		}, os.Stdout)
}

// PrintTable prints virtual services using tables to io.Writer
func VirtualServiceTable(list []*v1.VirtualService, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Virtual Service", "Display Name", "Domains", "SSL", "Status", "ListenerPlugins", "Routes"})

	for _, v := range list {
		name := v.GetMetadata().Name
		displayName := v.GetDisplayName()
		domains := domains(v)
		ssl := sslConfig(v)
		status := getStatus(v)
		routes := routeList(v.GetVirtualHost().GetRoutes())
		plugins := vhPlugins(v)

		if len(routes) == 0 {
			routes = []string{""}
		}
		for i, line := range routes {
			if i == 0 {
				table.Append([]string{name, displayName, domains, ssl, status, plugins, line})
			} else {
				table.Append([]string{"", "", "", "", "", "", line})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

// PrintTable prints virtual services using tables to io.Writer
func RouteTableTable(list []*v1.RouteTable, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Route Table", "Routes", "Status"})

	for _, rt := range list {
		name := rt.GetMetadata().Name
		routes := routeList(rt.GetRoutes())
		status := getRouteTableStatus(rt)

		if len(routes) == 0 {
			routes = []string{""}
		}
		for i, line := range routes {
			if i == 0 {
				table.Append([]string{name, line, status})
			} else {
				table.Append([]string{"", line, ""})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func getRouteTableStatus(vs *v1.RouteTable) string {

	// If the virtual service has not yet been accepted, don't clutter the status with the other errors.
	resourceStatus := vs.Status.State
	if resourceStatus != core.Status_Accepted {
		return resourceStatus.String()
	}

	// Subresource statuses are reported as a map[string]*Status
	// At the moment, virtual services only have one subresource, the associated gateway.
	// In the future, we may add more.
	// Either way, we only care if a subresource is in a non-accepted state.
	// Therefore, only report non-accepted states, include the subresource name.
	subResourceErrorMessages := []string{}
	for k, v := range vs.Status.SubresourceStatuses {
		if v.State != core.Status_Accepted {
			subResourceErrorMessages = append(subResourceErrorMessages, fmt.Sprintf("%v %v: %v", k, v.State.String(), v.Reason))
		}
	}

	switch len(subResourceErrorMessages) {
	case 0:
		// there are no errors with the subresources, pass Accepted status
		return resourceStatus.String()
	case 1:
		// there is one error, try to pass a friendly error message
		return cleanSubResourceError(subResourceErrorMessages[0])
	default:
		// there are multiple errors, don't be fancy, just return list
		return strings.Join(subResourceErrorMessages, "\n")
	}
}

func getStatus(res resources.InputResource) string {

	// If the virtual service has not yet been accepted, don't clutter the status with the other errors.
	resourceStatus := res.GetStatus().State
	if resourceStatus != core.Status_Accepted {
		return resourceStatus.String()
	}

	// Subresource statuses are reported as a map[string]*Status
	// At the moment, virtual services only have one subresource, the associated gateway.
	// In the future, we may add more.
	// Either way, we only care if a subresource is in a non-accepted state.
	// Therefore, only report non-accepted states, include the subresource name.
	subResourceErrorMessages := []string{}
	for k, v := range res.GetStatus().SubresourceStatuses {
		if v.State != core.Status_Accepted {
			subResourceErrorMessages = append(subResourceErrorMessages, fmt.Sprintf("%v %v: %v", k, v.State.String(), v.Reason))
		}
	}

	switch len(subResourceErrorMessages) {
	case 0:
		// there are no errors with the subresources, pass Accepted status
		return resourceStatus.String()
	case 1:
		// there is one error, try to pass a friendly error message
		return cleanSubResourceError(subResourceErrorMessages[0])
	default:
		// there are multiple errors, don't be fancy, just return list
		return strings.Join(subResourceErrorMessages, "\n")
	}
}

// If we can identify the type of error on a virtual service subresource,
// return a cleaner message. If not, default to the full error message.
func cleanSubResourceError(eMsg string) string {
	// If we add additional error scrubbers, we should use regexs
	// For now, a simple way to test for the known error is to split the full error message by it
	// If the split produced a list with two elements, then the error message is recognized
	parts := strings.Split(eMsg, gloov1.UpstreamListErrorTag)
	if len(parts) == 2 {
		// if here, eMsg ~= "<preamble><well_known_error_string><error_details>"
		errorDetails := parts[1]
		return subResourceErrorFormat(errorDetails)
	}
	return eMsg
}

func routeList(routeList []*v1.Route) []string {
	if len(routeList) == 0 {
		return nil
	}
	var routes []string
	for _, route := range routeList {
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

func destinationString(route *v1.Route) string {
	switch action := route.Action.(type) {
	case *v1.Route_RouteAction:
		switch dest := action.RouteAction.Destination.(type) {
		case *gloov1.RouteAction_Multi:
			return fmt.Sprintf("%v destinations", len(dest.Multi.Destinations))
		case *gloov1.RouteAction_Single:
			switch destType := dest.Single.DestinationType.(type) {
			case *gloov1.Destination_Upstream:
				return fmt.Sprintf("%s (upstream)", destType.Upstream.Key())
			case *gloov1.Destination_Kube:
				return fmt.Sprintf("%s (service)", destType.Kube.Ref.Key())
			}
		case *gloov1.RouteAction_UpstreamGroup:
			return fmt.Sprintf("upstream group: %s.%s", dest.UpstreamGroup.Name, dest.UpstreamGroup.Namespace)
		}
	case *v1.Route_DirectResponseAction:
		return strconv.Itoa(int(action.DirectResponseAction.Status))
	case *v1.Route_RedirectAction:
		return action.RedirectAction.HostRedirect
	case *v1.Route_DelegateAction:
		return fmt.Sprintf("%s (route table)", action.DelegateAction.Key())

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

func genericErrorFormat(resourceName, statusString, reason string) string {
	return fmt.Sprintf("%v %v: %v",
		strings.TrimSpace(resourceName),
		strings.TrimSpace(statusString),
		strings.TrimSpace(reason))
}
func subResourceErrorFormat(errorDetails string) string {
	return fmt.Sprintf("Error with Route: %v: %v", strings.TrimSpace(gloov1.UpstreamListErrorTag), strings.TrimPrefix(errorDetails, ": "))
}

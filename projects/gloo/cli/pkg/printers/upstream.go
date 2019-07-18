package printers

import (
	"fmt"
	"io"
	"sort"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// PrintTable prints virtual services using tables to io.Writer
func UpstreamTable(upstreams []*v1.Upstream, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Upstream", "type", "status", "details"})

	for _, us := range upstreams {
		name := us.GetMetadata().Name
		s := us.Status.State.String()

		u := "Unknown"
		details := []string{"Invalid: Spec was nil"}
		if us.UpstreamSpec != nil {
			u = upstreamType(us)
			details = upstreamDetails(us)
		}

		if len(details) == 0 {
			details = []string{""}
		}
		for i, line := range details {
			if i == 0 {
				table.Append([]string{name, u, s, line})
			} else {
				table.Append([]string{"", "", "", line})
			}
		}

	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func upstreamType(up *v1.Upstream) string {
	switch up.UpstreamSpec.UpstreamType.(type) {
	case *v1.UpstreamSpec_Aws:
		return "AWS"
	case *v1.UpstreamSpec_Azure:
		return "Azure"
	case *v1.UpstreamSpec_Consul:
		return "Consul"
	case *v1.UpstreamSpec_Kube:
		return "Kubernetes"
	case *v1.UpstreamSpec_Static:
		return "Static"
	default:
		return "Unknown"
	}
}

func upstreamDetails(up *v1.Upstream) []string {
	var details []string
	add := func(s ...string) {
		details = append(details, s...)
	}
	switch usType := up.UpstreamSpec.UpstreamType.(type) {
	case *v1.UpstreamSpec_Aws:
		var functions []string
		for _, fn := range usType.Aws.LambdaFunctions {
			functions = append(functions, fn.LambdaFunctionName)
		}
		add(
			fmt.Sprintf("region: %v", usType.Aws.Region),
			fmt.Sprintf("secret: %v", usType.Aws.SecretRef.Key()),
		)
		for i := range functions {
			if i == 0 {
				add("functions:")
			}
			add(fmt.Sprintf("- %v", functions[i]))
		}
	case *v1.UpstreamSpec_Azure:
		var functions []string
		for _, fn := range usType.Azure.Functions {
			functions = append(functions, fn.FunctionName)
		}
		add(
			fmt.Sprintf("function app name: %v", usType.Azure.FunctionAppName),
			fmt.Sprintf("secret: %v", usType.Azure.SecretRef.Key()),
		)

		for i := range functions {
			if i == 0 {
				add("functions:")
			}
			add(fmt.Sprintf("- %v", functions[i]))
		}
	case *v1.UpstreamSpec_Consul:
		add(
			fmt.Sprintf("svc name: %v", usType.Consul.ServiceName),
			fmt.Sprintf("svc tags: %v", usType.Consul.ServiceTags),
		)
		if usType.Consul.ServiceSpec != nil {
			add(linesForServiceSpec(usType.Consul.ServiceSpec)...)
		}
	case *v1.UpstreamSpec_Kube:
		add(
			fmt.Sprintf("svc name:      %v", usType.Kube.ServiceName),
			fmt.Sprintf("svc namespace: %v", usType.Kube.ServiceNamespace),
			fmt.Sprintf("port:          %v", usType.Kube.ServicePort),
		)
		if usType.Kube.ServiceSpec != nil {
			add(linesForServiceSpec(usType.Kube.ServiceSpec)...)
		}
	case *v1.UpstreamSpec_Static:
		for i := range usType.Static.Hosts {
			if i == 0 {
				add("hosts:")
			}
			add(fmt.Sprintf("- %v:%v", usType.Static.Hosts[i].Addr, usType.Static.Hosts[i].Port))
		}
		if usType.Static.ServiceSpec != nil {
			add(linesForServiceSpec(usType.Static.ServiceSpec)...)
		}
	}
	add("")
	return details
}

func linesForServiceSpec(serviceSpec *plugins.ServiceSpec) []string {
	var spec []string
	add := func(s ...string) {
		spec = append(spec, s...)
	}
	switch plug := serviceSpec.PluginType.(type) {
	case *plugins.ServiceSpec_Rest:
		add("REST service:")
		var functions []string
		for restFunc, transform := range plug.Rest.Transformations {
			var method, path string
			methodP, ok := transform.Headers[":method"]
			if ok {
				method = methodP.Text
			}
			pathP, ok := transform.Headers[":path"]
			if ok {
				path = pathP.Text
			}
			if false {
				//TODO ilackarms: save it for -o wide
				functions = append(functions, fmt.Sprintf("- %v (%v %v)", restFunc, method, path))
			}
			functions = append(functions, fmt.Sprintf("- %v", restFunc))
		}
		// needed because map
		sort.Strings(functions)

		for i := range functions {
			if i == 0 {
				add("functions:")
			}
			add(functions[i])
		}
	case *plugins.ServiceSpec_Grpc:
		add("gRPC service:")
		for _, grpcService := range plug.Grpc.GrpcServices {
			add(fmt.Sprintf("  %v", grpcService.ServiceName))
			for _, fn := range grpcService.FunctionNames {
				add(fmt.Sprintf("  - %v", fn))
			}
		}
	}

	return spec
}

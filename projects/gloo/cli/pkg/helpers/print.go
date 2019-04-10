package helpers

import (
	"io"
	"os"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
)

func PrintUpstreams(upstreams v1.UpstreamList, outputType string) {
	cliutils.PrintList(outputType, "", upstreams,
		func(data interface{}, w io.Writer) error {
			printers.UpstreamTable(data.(v1.UpstreamList), w)
			return nil
		}, os.Stdout)
}

func PrintProxies(proxies v1.ProxyList, outputType string) {
	cliutils.PrintList(outputType, "", proxies,
		func(data interface{}, w io.Writer) error {
			printers.ProxyTable(data.(v1.ProxyList), w)
			return nil
		}, os.Stdout)
}

func PrintVirtualServices(virtualServices gatewayv1.VirtualServiceList, outputType string) {
	cliutils.PrintList(outputType, "", virtualServices,
		func(data interface{}, w io.Writer) error {
			printers.VirtualServiceTable(data.(gatewayv1.VirtualServiceList), w)
			return nil
		}, os.Stdout)
}

func PrintRoutes(routes []*v1.Route, outputType string) {
	cliutils.PrintList(outputType, "", routes,
		func(data interface{}, w io.Writer) error {
			printers.RouteTable(data.([]*v1.Route), w)
			return nil
		}, os.Stdout)
}

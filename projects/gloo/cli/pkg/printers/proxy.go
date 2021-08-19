package printers

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
)

func PrintProxies(proxies v1.ProxyList, outputType OutputType) error {
	if outputType == KUBE_YAML {
		return PrintKubeCrdList(proxies.AsInputResources(), v1.ProxyCrd)
	}
	return cliutils.PrintList(outputType.String(), "", proxies,
		func(data interface{}, w io.Writer) error {
			ProxyTable(data.(v1.ProxyList), w)
			return nil
		}, os.Stdout)
}

// PrintTable prints proxies using tables to io.Writer
func ProxyTable(list v1.ProxyList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Proxy", "Listeners", "Virtual Hosts", "Status"})

	for _, proxy := range list {
		var (
			listeners []string
			vhCount   int
		)
		for _, listener := range proxy.GetListeners() {
			listeners = append(listeners, fmt.Sprintf("%v:%v", listener.GetBindAddress(), listener.GetBindPort()))
			http, ok := listener.GetListenerType().(*v1.Listener_HttpListener)
			if !ok {
				continue
			}
			vhCount += len(http.HttpListener.GetVirtualHosts())
		}
		name := proxy.GetMetadata().GetName()

		if len(listeners) == 0 {
			listeners = []string{""}
		}
		for i, listener := range listeners {
			if i == 0 {
				table.Append([]string{name, listener, strconv.Itoa(vhCount), proxy.GetStatus().GetState().String()})
			} else {
				table.Append([]string{"", listener, "", ""})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

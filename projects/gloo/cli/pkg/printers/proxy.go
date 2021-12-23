package printers

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

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
			switch listenerType := listener.GetListenerType().(type) {
			case *v1.Listener_HttpListener:
				vhCount += len(listenerType.HttpListener.GetVirtualHosts())
			case *v1.Listener_HybridListener:
				vhostMap := map[*v1.VirtualHost]struct{}{}
				for _, matchedListener := range listenerType.HybridListener.GetMatchedListeners() {
					httpListener := matchedListener.GetHttpListener()
					if httpListener == nil {
						continue
					}
					for _, vh := range httpListener.GetVirtualHosts() {
						vhostMap[vh] = struct{}{}
					}
				}
				vhCount += len(vhostMap)
			}
		}
		name := proxy.GetMetadata().GetName()

		if len(listeners) == 0 {
			listeners = []string{""}
		}
		for i, listener := range listeners {
			if i == 0 {
				table.Append([]string{name, listener, strconv.Itoa(vhCount), getAggregateProxyStatus(proxy)})
			} else {
				table.Append([]string{"", listener, "", ""})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func getAggregateProxyStatus(res resources.InputResource) string {
	return AggregateNamespacedStatuses(res.GetNamespacedStatuses(), func(status *core.Status) string {
		return status.GetState().String()
	})
}

package printers

import (
	"fmt"
	"io"
	"strconv"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// PrintTable prints proxies using tables to io.Writer
func ProxyTable(list v1.ProxyList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Proxy", "Listeners", "Virtual Hosts", "Status"})

	for _, proxy := range list {
		var (
			listeners []string
			vhCount   int
		)
		for _, listener := range proxy.Listeners {
			listeners = append(listeners, fmt.Sprintf("%v:%v", listener.BindAddress, listener.BindPort))
			http, ok := listener.ListenerType.(*v1.Listener_HttpListener)
			if !ok {
				continue
			}
			vhCount += len(http.HttpListener.VirtualHosts)
		}
		name := proxy.GetMetadata().Name

		if len(listeners) == 0 {
			listeners = []string{""}
		}
		for i, listener := range listeners {
			if i == 0 {
				table.Append([]string{name, listener, strconv.Itoa(vhCount), proxy.Status.State.String()})
			} else {
				table.Append([]string{"", listener, "", ""})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

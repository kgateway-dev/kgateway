package printers

import (
	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"io"
)

// PrintTable prints upstream groups using tables to io.Writer
func UpstreamGroupTable(upstreams []*v1.UpstreamGroup, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"UpstreamGroup", "status", "details"})

	for _, us := range upstreams {
		name := us.GetMetadata().Name
		s := us.Status.State.String()

		//u := upstreamType(us)
		//details := upstreamDetails(us) //TODO finish
		details := []string{""}

		if len(details) == 0 {
			details = []string{""}
		}
		for i, line := range details {
			if i == 0 {
				table.Append([]string{name, s, line})
			} else {
				table.Append([]string{"", "", line})
			}
		}

	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}
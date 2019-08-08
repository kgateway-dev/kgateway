package printers

import (
	"fmt"
	"io"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers/types"

	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
)

func PrintUpstreamGroups(upstreamGroups v1.UpstreamGroupList, outputType types.OutputType) error {
	if outputType == types.KUBE_YAML {
		return PrintKubeCrdList(upstreamGroups.AsInputResources(), v1.UpstreamGroupCrd)
	}
	return cliutils.PrintList(outputType.String(), "", upstreamGroups,
		func(data interface{}, w io.Writer) error {
			UpstreamGroupTable(data.(v1.UpstreamGroupList), w)
			return nil
		}, os.Stdout)
}

// PrintTable prints upstream groups using tables to io.Writer
func UpstreamGroupTable(upstreamGroups []*v1.UpstreamGroup, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Upstream Group", "status", "total weight", "details"})

	for i, ug := range upstreamGroups {
		name := ug.GetMetadata().Name
		status := ug.Status.State.String()

		weight := fmt.Sprint(totalWeight(ug))
		details := upstreamGroupDetails(ug)

		if len(details) == 0 {
			details = []string{""}
		}
		for j, line := range details {
			if j == 0 {
				table.Append([]string{name, status, weight, line})
			} else {
				table.Append([]string{"", "", "", line})
			}
		}
		if i != len(upstreamGroups)-1 {
			table.Append([]string{"", "", "", "---"})
		}

	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func totalWeight(ug *v1.UpstreamGroup) uint32 {
	weight := uint32(0)
	for _, us := range ug.Destinations {
		weight += us.Weight
	}
	return weight
}

func upstreamGroupDetails(ug *v1.UpstreamGroup) []string {
	var details []string
	add := func(s ...string) {
		details = append(details, s...)
	}
	totalWeight := totalWeight(ug)
	for i, us := range ug.Destinations {
		if i != 0 {
			add(fmt.Sprintf("\n"))
		}
		switch dest := us.Destination.DestinationType.(type) {
		case *v1.Destination_Upstream:
			add(fmt.Sprintf("destination type: %v", "Upstream"))
			add(fmt.Sprintf("namespace: %v", dest.Upstream.Namespace))
			add(fmt.Sprintf("name: %v", dest.Upstream.Name))
		case *v1.Destination_Kube:
			add(fmt.Sprintf("destination type: %v", "Kube"))
			add(fmt.Sprintf("namespace: %v", dest.Kube.Ref.Namespace))
			add(fmt.Sprintf("name: %v", dest.Kube.Ref.Name))
		case *v1.Destination_Consul:
			add(fmt.Sprintf("destination type: %v", "Consul"))
			add(fmt.Sprintf("service name: %v", dest.Consul.ServiceName))
			add(fmt.Sprintf("data centers: %v", dest.Consul.DataCenters))
			add(fmt.Sprintf("tags: %v", dest.Consul.Tags))
		default:
			add(fmt.Sprintf("destination type: %v", "Unknown"))
		}

		if us.Destination.Subset != nil {
			add(fmt.Sprintf("subset: %v", us.Destination.Subset.Values))
		}

		add(fmt.Sprintf("weight: %v   %% total: %.2f", us.Weight, float32(us.Weight)/float32(totalWeight)))
	}
	return details
}

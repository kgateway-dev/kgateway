package printers

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	ratelimit "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	"github.com/solo-io/go-utils/cliutils"
	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
)

func PrintRateLimitConfigs(ratelimitConfigs ratelimit.RateLimitConfigList, outputType OutputType) error {
	if outputType == KUBE_YAML || outputType == YAML {
		return PrintKubeCrdList(ratelimitConfigs.AsInputResources(), ratelimit.RateLimitConfigCrd)
	}
	return cliutils.PrintList(outputType.String(), "", ratelimitConfigs,
		func(data interface{}, w io.Writer) error {
			RateLimitConfig(data.(ratelimit.RateLimitConfigList), w)
			return nil
		}, os.Stdout)
}

// prints RateLimitConfigs using tables to io.Writer
func RateLimitConfig(list ratelimit.RateLimitConfigList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"RateLimitConfig", "Descriptors", "SetDescriptors"})

	for _, ratelimitConfig := range list {
		name := ratelimitConfig.GetMetadata().Name
		table.Append([]string{name, printDescriptors(ratelimitConfig.Spec.GetRaw().GetDescriptors()),
			printSetDescriptors(ratelimitConfig.Spec.GetRaw().GetSetDescriptors())})
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func printDescriptors(descriptors []*rltypes.Descriptor) string {
	var b strings.Builder
	for _, descriptor := range descriptors {
		if descriptor.GetValue() != "" {
			fmt.Fprintf(&b, "- %s = %s\n", descriptor.GetKey(), descriptor.GetValue())
		} else {
			fmt.Fprintf(&b, "- %s\n", descriptor.GetKey())
		}
		if len(descriptor.GetDescriptors()) != 0 {
			fmt.Fprint(&b, printDescriptors(descriptor.GetDescriptors()))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func printSetDescriptors(setDescriptors []*rltypes.SetDescriptor) string {
	var b strings.Builder
	for _, setDescriptor := range setDescriptors {
		for _, simpleDescriptor := range setDescriptor.GetSimpleDescriptors() {
			if simpleDescriptor.GetValue() != "" {
				fmt.Fprintf(&b, "- %s  =%s\n", simpleDescriptor.GetKey(), simpleDescriptor.GetValue())
			} else {
				fmt.Fprintf(&b, "- %s\n", simpleDescriptor.GetKey())
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

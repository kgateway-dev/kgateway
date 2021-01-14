package printers

import (
	"fmt"
	"io"
	"os"

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
			SetRateLimitConfig(data.(ratelimit.RateLimitConfigList), w)
			return nil
		}, os.Stdout)
}

// prints RateLimitConfigs using tables to io.Writer
func RateLimitConfig(list ratelimit.RateLimitConfigList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"RateLimitConfig", "Descriptors", "Actions"})
	for _, ratelimitConfig := range list {
		name := ratelimitConfig.GetMetadata().Name
		actions := printActions(ratelimitConfig.Spec.GetRaw().GetRateLimits())
		maxNumLines := len(actions)
		descriptors := printDescriptors(ratelimitConfig.Spec.GetRaw().GetDescriptors())
		if len(descriptors) > maxNumLines {
			maxNumLines = len(descriptors)
		}

		for i := 0; i < maxNumLines; i++ {
			var a, d = "", ""
			if i < len(descriptors) {
				d = descriptors[i]
			}
			if i < len(actions) {
				a = actions[i]
			}
			if i == 0 {
				if maxNumLines == 1 && a == "" && d == "" {
					continue
				}
				table.Append([]string{name, d, a})
			} else {
				table.Append([]string{"", d, a})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func SetRateLimitConfig(list ratelimit.RateLimitConfigList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Set-style RateLimitConfig", "Set Descriptors", "Set Actions"})

	for _, ratelimitConfig := range list {
		name := ratelimitConfig.GetMetadata().Name
		setActions := printSetActions(ratelimitConfig.Spec.GetRaw().GetRateLimits())
		maxNumLines := len(setActions)
		setDescriptors := printSetDescriptors(ratelimitConfig.Spec.GetRaw().GetSetDescriptors())
		if len(setDescriptors) > maxNumLines {
			maxNumLines = len(setDescriptors)
		}
		for i := 0; i < maxNumLines; i++ {
			var sa, sd = "", ""
			if i < len(setDescriptors) {
				sd = setDescriptors[i]
			}
			if i < len(setActions) {
				sa = setActions[i]
			}
			if i == 0 {
				if maxNumLines == 1 && sd == "" && sa == "" {
					continue
				}
				table.Append([]string{name, sd, sa})
			} else {
				table.Append([]string{"", sd, sa})
			}
		}
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

func printActions(ratelimitActions []*rltypes.RateLimitActions) []string {
	var actions []string
	for _, ratelimitAction := range ratelimitActions {
		for _, setAction := range ratelimitAction.GetActions() {
			actions = append(actions, setAction.String())
		}
	}
	actions = append(actions, "")
	return actions
}

func printSetActions(ratelimitActions []*rltypes.RateLimitActions) []string {
	var setActions []string
	for _, ratelimitAction := range ratelimitActions {
		for _, setAction := range ratelimitAction.GetSetActions() {
			setActions = append(setActions, setAction.String())
		}
	}
	setActions = append(setActions, "")
	return setActions
}

func printDescriptors(descriptors []*rltypes.Descriptor) []string {
	var res []string
	for _, descriptor := range descriptors {
		res = append(res, fmt.Sprintf("- %s", descriptor.String()))
	}
	res = append(res, "")
	return res
}

func printSetDescriptors(setDescriptors []*rltypes.SetDescriptor) []string {
	var res []string
	for _, setDescriptor := range setDescriptors {
		res = append(res, fmt.Sprintf("- requests_per_unit: %v", setDescriptor.GetRateLimit().GetRequestsPerUnit()))
		res = append(res, fmt.Sprintf("  unit: %v", setDescriptor.GetRateLimit().GetUnit()))
		res = append(res, fmt.Sprintf("  always_apply: %v", setDescriptor.GetAlwaysApply()))
		if len(setDescriptor.GetSimpleDescriptors()) > 0 {
			res = append(res, fmt.Sprintf("  simple_descriptors:"))
		}
		for _, simpleDescriptor := range setDescriptor.GetSimpleDescriptors() {
			if simpleDescriptor.GetValue() != "" {
				res = append(res, fmt.Sprintf("    - %s = %s", simpleDescriptor.GetKey(), simpleDescriptor.GetValue()))
			} else {
				res = append(res, fmt.Sprintf("    - %s", simpleDescriptor.GetKey()))
			}
		}
	}
	res = append(res, "")
	return res
}

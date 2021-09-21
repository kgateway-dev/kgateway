package printers

import (
	"fmt"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/ghodss/yaml"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

func PrintKubeCrd(in resources.InputResource, resourceCrd crd.Crd) error {
	str, err := GenerateKubeCrdString(in, resourceCrd)
	if err != nil {
		return err
	}
	fmt.Println(str)
	return nil
}

func GenerateKubeCrdString(in resources.InputResource, resourceCrd crd.Crd) (string, error) {
	res, err := resourceCrd.KubeResource(in)
	if err != nil {
		return "", err
	}
	raw, err := yaml.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func PrintKubeCrdList(in resources.InputResourceList, resourceCrd crd.Crd) error {
	for i, v := range in {
		if i != 0 {
			fmt.Print("\n --- \n")
		}
		if err := PrintKubeCrd(v, resourceCrd); err != nil {
			return err
		}
	}
	return nil
}

// AggregateNamespacedStatuses Formats a NamespacedStatuses into a string, using the statusProcessor function to
// format each individual controller's status
func AggregateNamespacedStatuses(namespacedStatuses *core.NamespacedStatuses, statusProcessor func(*core.Status) string) string {
	// If there are no statuses defined for this resource, default to a pending status
	if namespacedStatuses == nil {
		return core.Status_Pending.String()
	}

	// If there is more than one status, aggregate them
	if len(namespacedStatuses.GetStatuses()) > 1 {
		return aggregateMultipleNamespacedStatuses(namespacedStatuses, statusProcessor)
	}

	return aggregateSingleNamespacedStatus(namespacedStatuses, statusProcessor)
}

func aggregateSingleNamespacedStatus(namespacedStatuses *core.NamespacedStatuses, statusProcessor func(*core.Status) string) string {
	var sb strings.Builder

	for _, status := range namespacedStatuses.GetStatuses() {
		// for a resource with a single status, we don't need to include the controller information
		sb.WriteString(statusProcessor(status))
		// we expect there to only be one status in this map
		// but we short circuit anyway
		break
	}
	return sb.String()
}

func aggregateMultipleNamespacedStatuses(namespacedStatuses *core.NamespacedStatuses, statusProcessor func(*core.Status) string) string {
	var sb strings.Builder
	var index = 0

	for controller, status := range namespacedStatuses.GetStatuses() {
		sb.WriteString(controller)
		sb.WriteString(": ")
		sb.WriteString(statusProcessor(status))
		index += 1
		// Don't write newline after last status in the map
		if index != len(namespacedStatuses.GetStatuses()) {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

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
func AggregateNamespacedStatuses(NamespacedStatuses *core.NamespacedStatuses, statusProcessor func(*core.Status) string) string {
	var sb strings.Builder
	var index = 0
	for controller, status := range NamespacedStatuses.GetStatuses() {
		sb.WriteString(controller)
		sb.WriteString(": ")
		sb.WriteString(statusProcessor(status))
		index += 1
		// Don't write newline after last status in the map
		if index != len(NamespacedStatuses.GetStatuses()) {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

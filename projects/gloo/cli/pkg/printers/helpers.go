package printers

import (
	"fmt"

	"github.com/ghodss/yaml"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

func PrintKubeCrd(in resources.InputResource, resourceCrd crd.Crd) error {
	raw, err := yaml.Marshal(resourceCrd.KubeResource(in))
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
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

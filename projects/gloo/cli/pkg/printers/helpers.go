package printers

import (
	"fmt"

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

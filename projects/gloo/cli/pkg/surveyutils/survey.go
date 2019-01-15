package surveyutils

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	survey "gopkg.in/AlecAivazis/survey.v1"
)

func ResourceSelectMultiByNamespace(resources resources.ResourceList) (resources.ResourceList, error) {
	namespaces := []string{}
	prompt := &survey.MultiSelect{
		Message: "Which namespaces would you like to search?",
		Options: resources.Namespaces(),
	}
	err := survey.AskOne(prompt, &namespaces, nil)
	resourceList := resources.FilterByNamespaces(namespaces)

	return resourceList, err
}

func ResourceSelectByNamespace(resources resources.ResourceList) (resources.ResourceList, error) {
	var namespace string
	prompt := &survey.Select{
		Message: "Which namespaces would you like to search?",
		Options: resources.Namespaces(),
	}
	err := survey.AskOne(prompt, &namespace, nil)
	resourceList := resources.FilterByNamespaces([]string{namespace})

	return resourceList, err
}

func ResourceSelectMultiByName(resources resources.ResourceList) (resources.ResourceList, error) {
	resourceNames := []string{}
	prompt := &survey.MultiSelect{
		Message: "Which items would you like to act on?",
		Options: resources.Names(),
	}
	err := survey.AskOne(prompt, &resourceNames, nil)
	resourceList := resources.FilterByNames(resourceNames)

	return resourceList, err
}

func ResourceSelectByName(message string, resources resources.ResourceList) (resources.ResourceList, error) {
	var resourceName string
	prompt := &survey.Select{
		Message: message,
		Options: resources.Names(),
	}
	err := survey.AskOne(prompt, &resourceName, nil)
	resourceList := resources.FilterByNames([]string{resourceName})

	return resourceList, err
}

func EnsureResourceByName(message string, static bool, source string, target *resources.Resource, resources resources.ResourceList) error {
	resourceName := source
	if static && resourceName == "" {
		return fmt.Errorf("Please specify a resource name")
	}
	if !static {
		prompt := &survey.Select{
			Message: message,
			Options: resources.Names(),
		}
		if err := survey.AskOne(prompt, &resourceName, survey.Required); err != nil {
			return err
		}
	}
	resourceList := resources.FilterByNames([]string{resourceName})
	if len(resourceList) == 0 {
		return fmt.Errorf("Resource %v not found", resourceName)
	}
	*target = resourceList[0]
	return nil
}

func InteractiveNamespace(namespace *string) error {
	nsList := helpers.MustGetNamespaces()
	if err := cliutil.ChooseFromList("Please choose a namespace", namespace, nsList); err != nil {
		return err
	}
	return nil
}

package nsselect

import (
	"context"
	"fmt"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	survey "gopkg.in/AlecAivazis/survey.v1"
)

//NOTE these functions are good candidates for code generation

func ChooseResource(typeName string, menuDescription string, nsr NsResourceMap) (core.ResourceRef, error) {

	resOptions, resMap := generateCommonResourceSelectOptions(typeName, nsr)
	if len(resOptions) == 0 {
		return core.ResourceRef{}, fmt.Errorf("No %v found. Please create a %v", menuDescription, menuDescription)
	}
	question := &survey.Select{
		Message: fmt.Sprintf("Select a %v", menuDescription),
		Options: resOptions,
	}

	var choice string
	if err := cliutil.AskOne(question, &choice, survey.Required); err != nil {
		// this should not error
		fmt.Println("error with input")
		return core.ResourceRef{}, err
	}

	return resMap[choice].resourceRef, nil
}

// TODO(mitchdraft) merge with ChooseResource
func ChooseResources(typeName string, menuDescription string, nsr NsResourceMap) ([]*core.ResourceRef, error) {

	resOptions, resMap := generateCommonResourceSelectOptions(typeName, nsr)
	if len(resOptions) == 0 {
		return []*core.ResourceRef{}, fmt.Errorf("No %v found. Please create a %v", menuDescription, menuDescription)
	}
	question := &survey.MultiSelect{
		Message: fmt.Sprintf("Select a %v", menuDescription),
		Options: resOptions,
	}

	var choice []string
	if err := cliutil.AskOne(question, &choice, survey.Required); err != nil {
		// this should not error
		fmt.Println("error with input")
		return []*core.ResourceRef{}, err
	}
	var response []*core.ResourceRef
	for _, c := range choice {
		res := resMap[c].resourceRef
		response = append(response, &res)
	}

	return response, nil
}

// EnsureCommonResource validates a resRef relative to static vs. interactive mode
// If in interactive mode (non-static mode) and a resourceRef is not given, it will prompt the user to choose one
// This function works for multiple types of resources. Specify the resource type via typeName
// menuDescription - the string that the user will see when the prompt menu appears
func EnsureCommonResource(typeName string, menuDescription string, resRef *core.ResourceRef, nsrm NsResourceMap, static bool) error {
	if err := validateResourceRefForStaticMode(typeName, menuDescription, resRef, nsrm, static); err != nil {
		return err
	}

	// interactive mode
	if resRef.GetName() == "" || resRef.GetNamespace() == "" {
		chosenResRef, err := ChooseResource(typeName, menuDescription, nsrm)
		if err != nil {
			return err
		}
		*resRef = chosenResRef
	}
	return nil
}

// Static mode not supported ATM
// TODO(mitchdraft) integrate with static mode
func EnsureCommonResources(typeName string, menuDescription string, resRefs *[]*core.ResourceRef, nsrm NsResourceMap, static bool) error {
	chosenResRefs, err := ChooseResources(typeName, menuDescription, nsrm)
	if err != nil {
		return err
	}
	*resRefs = chosenResRefs
	return nil
}

func validateResourceRefForStaticMode(typeName string, menuDescription string, resRef *core.ResourceRef, nsrm NsResourceMap, static bool) error {
	if static {
		// make sure we have a full resource ref
		if resRef.GetName() == "" {
			return fmt.Errorf("Please provide a %v name", menuDescription)
		}
		if resRef.GetNamespace() == "" {
			return fmt.Errorf("Please provide a %v namespace", menuDescription)
		}

		// make sure they chose a valid namespace
		if _, ok := nsrm[resRef.GetNamespace()]; !ok {
			return fmt.Errorf("Please specify a valid namespace. Namespace %v not found.", resRef.GetNamespace())
		}

		// make sure that the particular resource exists in the specified namespace
		switch typeName {
		// case "secret":
		// 	if !cliutil.Contains(nsrm[resRef.Namespace].Secrets, resRef.Name) {
		// 		return fmt.Errorf("Please specify a valid %v name. %v not found in namespace %v.", resRef.Name, menuDescription, resRef.Namespace)
		// 	}
		case "upstream":
			if !cliutil.Contains(nsrm[resRef.GetNamespace()].Upstreams, resRef.GetName()) {
				return fmt.Errorf("Please specify a valid %v name. %v not found in namespace %v.", resRef.GetName(), menuDescription, resRef.GetNamespace())
			}
		default:
			contextutils.LoggerFrom(context.Background()).DPanic(fmt.Errorf("typename %v not recognized", typeName))
			return fmt.Errorf("typename %v not recognized", typeName)
		}
	}
	return nil
}

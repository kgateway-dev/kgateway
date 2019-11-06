package surveyutils

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

func AddVirtualServiceFlagsInteractive(vs *options.InputVirtualService) error {

	var msgProvider = func() string {
		return fmt.Sprintf("Add a domain for this virtual service (empty defaults to all domains)? Current domains %v", vs.Domains)
	}

	if err := cliutil.GetStringSliceInputLazyPrompt(msgProvider, &vs.Domains); err != nil {
		return err
	}

	if err := rateLimitingSurvey(&vs.RateLimit); err != nil {
		return err
	}

	if err := authConfigSurvey(&vs.AuthConfig); err != nil {
		return err
	}

	return nil
}

// TODO: move this to input virtual host
func rateLimitingSurvey(input *options.RateLimit) error {
	yes, err := cliutil.GetYesInput("do you wish to add rate limiting to the virtual service [y/n]?")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	input.Enable = true

	if err := cliutil.ChooseFromList(
		"Limit requests over this unit of time for clients: ",
		&input.TimeUnit,
		options.RateLimit_TimeUnits,
	); err != nil {
		return err
	}

	if err := cliutil.GetUint32InputDefault(
		fmt.Sprintf("how many requests per %v?", input.TimeUnit),
		&input.RequestsPerTimeUnit,
		100,
	); err != nil {
		return err
	}
	return nil
}

func authConfigSurvey(input *options.AuthConfig) error {
	yes, err := cliutil.GetYesInput("do you wish to add an auth config reference to the virtual host? [y/n]?")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	err = cliutil.GetStringInput("auth config namespace?", &input.Namespace)
	if err != nil {
		return err
	}

	err = cliutil.GetStringInput("auth config name?", &input.Name)
	if err != nil {
		return err
	}

	return nil
}

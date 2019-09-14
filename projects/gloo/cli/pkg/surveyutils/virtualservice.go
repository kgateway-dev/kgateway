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
	if err := oidcSurvey(&vs.OIDCAuth); err != nil {
		return err
	}
	if err := apiKeySurvey(&vs.ApiKeyAuth); err != nil {
		return err
	}

	if err := opaSurvey(&vs.OpaAuth); err != nil {
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

func oidcSurvey(input *options.OIDCAuth) error {
	yes, err := cliutil.GetYesInput("do you wish to add oidc auth to the virtual service [y/n]?")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	input.Enable = true

	err = cliutil.GetStringInput("What is your app url?", &input.AppUrl)
	if err != nil {
		return err
	}
	err = cliutil.GetStringInput("What is your issuer url?", &input.IssuerUrl)
	if err != nil {
		return err
	}
	err = cliutil.GetStringInputDefault("What path (relative to your app url) should we use as a callback from the issuer?", &input.CallbackPath, "/oidc-gloo-callback")
	if err != nil {
		return err
	}
	err = cliutil.GetStringInput("What is your client id?", &input.ClientId)
	if err != nil {
		return err
	}
	err = cliutil.GetStringInput("What is your client secret name?", &input.ClientSecretRef.Name)
	if err != nil {
		return err
	}
	err = cliutil.GetStringInput("What is your client secret namespace?", &input.ClientSecretRef.Namespace)
	if err != nil {
		return err
	}
	err = cliutil.GetStringSliceInput("provide additional scopes to request (empty to finish)", &input.Scopes)
	if err != nil {
		return err
	}

	return nil
}

func apiKeySurvey(input *options.ApiKeyAuth) error {
	yes, err := cliutil.GetYesInput("do you wish to add apikey auth to the virtual service [y/n]?")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	input.Enable = true

	err = cliutil.GetStringSliceInput("provide a label (key=value) to be a part of your label selector (empty to finish)", &input.Labels)
	if err != nil {
		return err
	}

	err = cliutil.GetStringInput("apikey secret name to attach to this virtual service? (empty to skip)", &input.SecretName)
	if err != nil {
		return err
	}

	if input.SecretName != "" {
		err = cliutil.GetStringInput("provide a namespace to search for the secret in (empty to finish)", &input.SecretNamespace)
		if err != nil {
			return err
		}
	}

	return nil
}

func opaSurvey(input *options.OpaAuth) error {
	yes, err := cliutil.GetYesInput("do you wish to add OPA auth to the virtual service [y/n]?")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	input.Enable = true

	err = cliutil.GetStringInput("OPA query to attach to this virtual service?", &input.Query)
	if err != nil {
		return err
	}
	if input.Query == "" {
		return fmt.Errorf("query must not be empty")
	}

	err = cliutil.GetStringSliceInput("provide references to config maps used as OPA modules in resolving above query (empty to finish)", &input.Modules)
	if err != nil {
		return err
	}

	return nil
}

package surveyutils

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

func AddAuthConfigFlagsInteractive(ac *options.InputAuthConfig) error {
	if err := oidcSurvey(&ac.OIDCAuth); err != nil {
		return err
	}
	if err := apiKeySurvey(&ac.ApiKeyAuth); err != nil {
		return err
	}

	if err := opaSurvey(&ac.OpaAuth); err != nil {
		return err
	}

	return nil
}

func oidcSurvey(input *options.OIDCAuth) error {
	yes, err := cliutil.GetYesInput("do you wish to add oidc auth to the auth config [y/n]?")
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
	// TODO(awang) add issuer query params
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
	yes, err := cliutil.GetYesInput("do you wish to add apikey auth to the auth config [y/n]?")
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

	err = cliutil.GetStringInput("apikey secret name to attach to this auth config? (empty to skip)", &input.SecretName)
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
	yes, err := cliutil.GetYesInput("do you wish to add OPA auth to the auth config [y/n]?")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	input.Enable = true

	err = cliutil.GetStringInput("OPA query to attach to this auth config?", &input.Query)
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

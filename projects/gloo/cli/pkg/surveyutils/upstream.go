package surveyutils

import (
	"context"
	"fmt"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func getAwsInteractive(ctx context.Context, aws *options.InputAwsSpec) error {
	if err := cliutil.GetStringInputDefault(
		"What region are the AWS services in for this upstream?",
		&aws.Region,
		"us-east-1",
	); err != nil {
		return err
	}

	// collect secrets list
	secretClient := helpers.MustSecretClient(ctx)
	secretsByKey := make(map[string]*core.ResourceRef)
	var secretKeys []string
	for _, ns := range helpers.MustGetNamespaces(ctx) {
		secretList, err := secretClient.List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, secret := range secretList {
			if _, ok := secret.GetKind().(*v1.Secret_Aws); !ok {
				continue
			}
			ref := secret.GetMetadata().Ref()
			secretsByKey[ref.Key()] = ref
			secretKeys = append(secretKeys, ref.Key())
		}
	}
	if len(secretKeys) == 0 {
		return errors.Errorf("no AWS secrets found. create an AWS credentials secret using " +
			"glooctl create secret aws --help")
	}
	var secretKey string
	if err := cliutil.ChooseFromList(
		"Choose an AWS credentials secret to link to this upstream: ",
		&secretKey,
		secretKeys,
	); err != nil {
		return err
	}
	aws.Secret = *secretsByKey[secretKey]
	return nil
}

func getAzureInteractive(ctx context.Context, azure *options.InputAzureSpec) error {
	if err := cliutil.GetStringInputDefault(
		"What is the name of the Azure Functions app to associate with this upstream?",
		&azure.FunctionAppName,
		"",
	); err != nil {
		return err
	}

	// collect secrets list
	secretClient := helpers.MustSecretClient(ctx)
	secretsByKey := make(map[string]*core.ResourceRef)
	var secretKeys []string
	for _, ns := range helpers.MustGetNamespaces(ctx) {
		secretList, err := secretClient.List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, secret := range secretList {
			if _, ok := secret.GetKind().(*v1.Secret_Azure); !ok {
				continue
			}
			ref := secret.GetMetadata().Ref()
			secretsByKey[ref.Key()] = ref
			secretKeys = append(secretKeys, ref.Key())
		}
	}
	if len(secretKeys) == 0 {
		return errors.Errorf("no Azure secrets found. create an Azure credentials secret using " +
			"glooctl create secret azure --help")
	}
	var secretKey string
	if err := cliutil.ChooseFromList(
		"Choose an Azure credentials secret to link to this upstream: ",
		&secretKey,
		secretKeys,
	); err != nil {
		return err
	}
	azure.Secret = *secretsByKey[secretKey]
	return nil
}

func getStaticInteractive(static *options.InputStaticSpec) error {
	var upstreamMsgProvider = func() string {
		return fmt.Sprintf("Add another host for this upstream (empty to skip)? %v", static.Hosts)
	}
	if err := cliutil.GetStringSliceInputLazyPrompt(upstreamMsgProvider, &static.Hosts); err != nil {
		return err
	}
	return nil
}

func AddUpstreamFlagsInteractive(ctx context.Context, upstream *options.InputUpstream) error {
	if upstream.UpstreamType == "" {
		if err := cliutil.ChooseFromList(
			"What type of Upstream do you want to create?",
			&upstream.UpstreamType,
			options.UpstreamTypes,
		); err != nil {
			return err
		}
	}
	switch upstream.UpstreamType {
	case options.UpstreamType_Aws:
		if err := getAwsInteractive(ctx, &upstream.Aws); err != nil {
			return err
		}
	case options.UpstreamType_Azure:
		if err := getAzureInteractive(ctx, &upstream.Azure); err != nil {
			return err
		}
	case options.UpstreamType_Static:
		if err := getStaticInteractive(&upstream.Static); err != nil {
			return err
		}
	case options.UpstreamType_Consul:
		fallthrough
	case options.UpstreamType_Kube:
		fallthrough
	default:
		return errors.Errorf("interactive mode not currently available for type %v", upstream.UpstreamType)
	}

	return nil
}

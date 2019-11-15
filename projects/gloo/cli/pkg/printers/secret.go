package printers

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ghodss/yaml"
	"github.com/olekukonko/tablewriter"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"k8s.io/client-go/kubernetes/fake"
)

func PrintSecrets(secrets v1.SecretList, outputType OutputType) error {
	if outputType == KUBE_YAML || outputType == YAML {
		return printKubeSecretList(context.TODO(), secrets.AsResources())
	}
	return cliutils.PrintList(outputType.String(), "", secrets,
		func(data interface{}, w io.Writer) error {
			SecretTable(data.(v1.SecretList), w)
			return nil
		}, os.Stdout)
}

// PrintTable prints secrets using tables to io.Writer
func SecretTable(list v1.SecretList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"Secret", "Type"})

	for _, secret := range list {
		var secretType string
		name := secret.GetMetadata().Name
		switch secret.Kind.(type) {
		case *v1.Secret_Aws:
			secretType = "AWS"
		case *v1.Secret_Azure:
			secretType = "Azure"
		case *v1.Secret_Tls:
			secretType = "TLS"
		case *v1.Secret_Oauth:
			secretType = "OAuth"
		case *v1.Secret_ApiKey:
			secretType = "ApiKey"
		default:
			secretType = "unknown"
		}

		table.Append([]string{name, secretType})
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}

// note: prints secrets in the traditional way, without using plain secrets or a custom secret converter
func printKubeSecret(ctx context.Context, in resources.Resource) error {
	baseSecretClient, err := secretBaseClient(ctx, in)
	if err != nil {
		return err
	}
	kubeSecret, err := baseSecretClient.ToKubeSecret(ctx, in)
	raw, err := yaml.Marshal(kubeSecret)
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func printKubeSecretList(ctx context.Context, in resources.ResourceList) error {
	for i, v := range in {
		if i != 0 {
			fmt.Print("\n --- \n")
		}
		if err := printKubeSecret(ctx, v); err != nil {
			return err
		}
	}
	return nil
}

func secretBaseClient(ctx context.Context, resourceType resources.Resource) (*kubesecret.ResourceClient, error) {
	clientset := fake.NewSimpleClientset()
	coreCache, err := cache.NewKubeCoreCache(ctx, clientset)
	if err != nil {
		return nil, err
	}
	return kubesecret.NewResourceClient(clientset, resourceType, false, coreCache)
}

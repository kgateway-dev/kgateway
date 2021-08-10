package printers

import (
	"context"
	"fmt"
	"io"
	"os"

	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	corev1 "k8s.io/api/core/v1"

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
		switch secret.GetKind().(type) {
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

func printKubeSecret(ctx context.Context, in resources.Resource) error {
	kubeSecret, err := toKubeSecret(ctx, in)
	if err != nil {
		return err
	}

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

func toKubeSecret(ctx context.Context, resource resources.Resource) (*corev1.Secret, error) {
	clientset := fake.NewSimpleClientset()
	coreCache, err := cache.NewKubeCoreCache(ctx, clientset)
	if err != nil {
		return nil, err
	}
	rc, err := kubesecret.NewResourceClient(clientset, resource, false, coreCache)
	if err != nil {
		return nil, err
	}

	// Try converters first
	secret, err := kubeconverters.GlooSecretConverterChain.ToKubeSecret(ctx, rc, resource)
	if err != nil || secret != nil {
		return secret, err
	}

	return rc.ToKubeSecret(ctx, resource)
}

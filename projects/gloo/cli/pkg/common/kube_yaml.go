package common

import (
	"context"
	"fmt"

	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/ghodss/yaml"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"k8s.io/client-go/kubernetes/fake"
)

func PrintYaml(in resources.Resource) error {
	raw, err := protoutils.MarshalYAML(in)
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

// note: prints secrets in the traditional way, without using plain secrets or a custom secret converter
func PrintKubeSecret(ctx context.Context, in resources.Resource) error {
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

func secretBaseClient(ctx context.Context, resourceType resources.Resource) (*kubesecret.ResourceClient, error) {
	clientset := fake.NewSimpleClientset()
	coreCache, err := cache.NewKubeCoreCache(ctx, clientset)
	if err != nil {
		return nil, err
	}
	return kubesecret.NewResourceClient(clientset, resourceType, false, coreCache)
}

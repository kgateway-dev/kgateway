package nsselect

import (
	"context"
	"fmt"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"k8s.io/client-go/rest"
)

// TODO(move this to a different place or use a different client function)
func GetUpstreamClient(ctx context.Context) (*gloov1.UpstreamClient, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, err
	}

	upstreamClient, err := gloov1.NewUpstreamClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         gloov1.UpstreamCrd,
		Cfg:         config,
		SharedCache: kube.NewKubeCache(context.TODO()),
	})
	if err != nil {
		return nil, err
	}
	if err = upstreamClient.Register(); err != nil {
		return nil, err
	}
	return &upstreamClient, nil
}

func GatherResources(ctx context.Context, namespaces []string) (NsResourceMap, error) {
	upstreamClient, err := GetUpstreamClient(ctx)
	if err != nil {
		return NsResourceMap{}, err
	}
	nsResources := make(map[string]*NsResource)
	for _, ns := range namespaces {
		// secretList, err := (*secretClient).List(ns, clients.ListOpts{})
		// if err != nil {
		// 	return err
		// }
		// var secrets = []string{}
		// for _, m := range secretList {
		// 	secrets = append(secrets, m.Metadata.Name)
		// }
		upstreamList, err := (*upstreamClient).List(ns, clients.ListOpts{})
		if err != nil {
			return NsResourceMap{}, err
		}
		var upstreams = []string{}
		for _, m := range upstreamList {
			upstreams = append(upstreams, m.GetMetadata().GetName())
		}

		nsResources[ns] = &NsResource{
			// Secrets:   secrets,
			Upstreams: upstreams,
		}
	}
	return nsResources, nil
}

// TODO(mitchdraft) move to common pkg
func getKubernetesConfig() (*rest.Config, error) {
	config, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, fmt.Errorf("Error with Kubernetese configuration: %v", err)
	}
	return config, nil
}

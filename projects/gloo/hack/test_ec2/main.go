package main

import (
	"context"
	"fmt"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"k8s.io/client-go/kubernetes"

	"github.com/pkg/errors"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("unable to run: %v", err)
	}
}

func run() error {
	ctx := context.Background()
	config, err := kubeutils.GetConfig("", "")
	if err != nil {
		return fmt.Errorf("Error with Kubernetese configuration: %v", err)
	}
	rc := &factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         config,
		SharedCache: kube.NewKubeCache(ctx),
	}
	upClient, err := v1.NewUpstreamClient(rc)
	if err != nil {
		return err
	}
	//upstream, err := upClient.Read("gloo-system", "mktestall", clients.ReadOpts{})
	//upstream, err := upClient.Read("gloo-system", "mktest", clients.ReadOpts{})
	upstream, err := upClient.Read("gloo-system", "mktestwip", clients.ReadOpts{})
	if err != nil {
		return err
	}

	mc := memory.NewInMemoryResourceCache()
	var clientSet kubernetes.Interface
	var kubeCoreCache corecache.KubeCoreCache
	settings := &v1.Settings{
		DiscoveryNamespace:   "",
		WatchNamespaces:      nil,
		ConfigSource:         nil,
		SecretSource:         &v1.Settings_KubernetesSecretSource{},
		ArtifactSource:       nil,
		BindAddr:             "",
		RefreshRate:          nil,
		DevMode:              false,
		Linkerd:              false,
		CircuitBreakers:      nil,
		Knative:              nil,
		Discovery:            nil,
		Extensions:           nil,
		Metadata:             core.Metadata{},
		Status:               core.Status{},
		XXX_NoUnkeyedLiteral: struct{}{},
		XXX_unrecognized:     nil,
		XXX_sizecache:        0,
	}
	sf, err := bootstrap.SecretFactoryForSettings(ctx, settings, mc, &config, &clientSet, &kubeCoreCache, v1.SecretCrd.Plural)
	if err != nil {
		return errors.Wrapf(err, "unable to create secret factory")
	}

	secretClient, err := v1.NewSecretClient(sf)
	if err != nil {
		return errors.Wrapf(err, "unable to create secret client")
	}
	secrets, err := secretClient.List("default", clients.ListOpts{})
	if err != nil {
		return err
	}
	fmt.Println("secrets are:")
	fmt.Println(secrets)
	ec2Upstream, ok := upstream.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return fmt.Errorf("invalid upstream: %v", upstream)
	}
	session, err := ec2.GetEc2Session(ec2Upstream.AwsEc2, secrets)
	if err != nil {
		return err
	}
	ec2InstancesForUpstream, err := ec2.ListEc2InstancesForCredentials(ctx, session, ec2Upstream.AwsEc2)
	if err != nil {
		return err
	}

	fmt.Println("ec2InstancesForUpstream")
	fmt.Println(ec2InstancesForUpstream)
	return nil

}

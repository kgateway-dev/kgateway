package cluster

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Context contains the metadata about a Kubernetes cluster
// It also includes useful utilities for interacting with that cluster
type Context struct {
	// The name of the Kubernetes cluster
	Name string

	// The context of the Kubernetes cluster
	KubeContext string

	// RestConfig holds the common attributes that can be passed to a Kubernetes client on initialization
	RestConfig *rest.Config

	// A CLI for interacting with Kubernetes cluster
	Cli *kubectl.Cli

	// A client to perform CRUD operations on the Kubernetes Cluster
	Client client.Client

	// A set of clients for interacting with the Kubernetes Cluster
	Clientset *kubernetes.Clientset

	// A set of clients for interacting with the Edge resources
	// TODO(npolshak): Add new clients as needed
	RouteOptionClient gatewayv1.RouteOptionClient
}

func (c *Context) AddRouteOptionClient(ctx context.Context) {
	cache := kube.NewKubeCache(ctx)
	routeOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteOptionCrd,
		Cfg:         c.RestConfig,
		SharedCache: cache,
	}
	routeOptionClient, err := gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	c.RouteOptionClient = routeOptionClient
}

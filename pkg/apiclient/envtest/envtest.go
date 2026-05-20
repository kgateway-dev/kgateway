// Package envtest provides an apiclient.Client implementation backed by envtest.
package envtest

import (
	"context"
	"fmt"
	"strings"

	"istio.io/istio/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	_ "istio.io/istio/pkg/kube/kclient" // Register NewCrdWatcher

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var _ apiclient.Client = (*envTestClient)(nil)

type envTestClient struct {
	kube.Client
	kgateway versioned.Interface
}

// NewClient creates a new envtest-backed client with the given objects pre-populated.
func NewClient(restCfg *rest.Config, objects ...client.Object) (apiclient.Client, error) {
	kubeClient, err := kube.NewClient(kube.NewClientConfigForRestConfig(restCfg), "")
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %w", err)
	}

	// Enable CRD watcher (required for NewDelayedInformer)
	kube.EnableCrdWatcher(kubeClient)

	kgwClient, err := versioned.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kgateway clientset: %w", err)
	}

	// Create a REST mapper using the discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	apiclient.RegisterTypes()

	cli := &envTestClient{
		Client:   kubeClient,
		kgateway: kgwClient,
	}

	// Create or update objects in the API server (upsert semantics)
	ctx := context.Background()
	for _, obj := range objects {
		ns := obj.GetNamespace()
		if ns != "" {
			// Ensure namespace exists
			_, err := kubeClient.Kube().CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
			if err != nil {
				_, createErr := kubeClient.Kube().CoreV1().Namespaces().Create(ctx, newNamespace(ns), metav1.CreateOptions{})
				if createErr != nil && !isAlreadyExists(createErr) {
					return nil, fmt.Errorf("failed to create namespace %s: %w", ns, createErr)
				}
			}
		}

		// Create the object using dynamic client for flexibility
		us, err := kubeutils.ToUnstructured(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %T to unstructured: %w", obj, err)
		}

		gvk := us.GroupVersionKind()
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST mapping for %v: %w", gvk, err)
		}

		dyn := kubeClient.Dynamic().Resource(mapping.Resource)
		name := us.GetName()

		// Try to create first
		if ns != "" {
			_, err = dyn.Namespace(ns).Create(ctx, us, metav1.CreateOptions{})
		} else {
			_, err = dyn.Create(ctx, us, metav1.CreateOptions{})
		}

		// If already exists, update instead (upsert semantics)
		if isAlreadyExists(err) {
			var existing *unstructured.Unstructured
			if ns != "" {
				existing, err = dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			} else {
				existing, err = dyn.Get(ctx, name, metav1.GetOptions{})
			}
			if err != nil {
				return nil, fmt.Errorf("failed to get existing %T %s for update: %w", obj, kubeutils.NamespacedNameFrom(obj), err)
			}

			// Copy resourceVersion to the new object for update
			us.SetResourceVersion(existing.GetResourceVersion())

			if ns != "" {
				_, err = dyn.Namespace(ns).Update(ctx, us, metav1.UpdateOptions{})
			} else {
				_, err = dyn.Update(ctx, us, metav1.UpdateOptions{})
			}
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create/update %T %s: %w", obj, kubeutils.NamespacedNameFrom(obj), err)
		}
	}

	return cli, nil
}

func (c *envTestClient) Kgateway() versioned.Interface {
	return c.kgateway
}

func (c *envTestClient) Core() kube.Client {
	return c.Client
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func isAlreadyExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

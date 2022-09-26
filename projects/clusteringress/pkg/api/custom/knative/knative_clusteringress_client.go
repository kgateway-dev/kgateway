package knative

import (
	"context"
	"fmt"
	"sort"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/clusteringress/api/external/knative"
	v1alpha1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/external/knative"
	knativev1alpha1 "knative.dev/networking/pkg/apis/networking/v1alpha1"
	knativeclient "knative.dev/networking/pkg/client/clientset/versioned"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ResourceClient struct {
	knativeClient knativeclient.Interface
	cache         Cache
}

func NewResourceClient(knativeClient knativeclient.Interface, cache Cache) *ResourceClient {
	return &ResourceClient{
		knativeClient: knativeClient,
		cache:         cache,
	}
}

func FromKube(ci *knativev1alpha1.Ingress) *v1alpha1.ClusterIngress {
	deepCopy := ci.DeepCopy()
	baseType := knative.ClusterIngress(*deepCopy)
	resource := &v1alpha1.ClusterIngress{
		ClusterIngress: baseType,
	}

	return resource
}

func ToKube(resource resources.Resource) (*knativev1alpha1.Ingress, error) {
	clusterIngressResource, ok := resource.(*v1alpha1.ClusterIngress)
	if !ok {
		return nil, errors.Errorf("internal error: invalid resource %v passed to clusteringress client", resources.Kind(resource))
	}

	clusterIngress := knativev1alpha1.Ingress(clusterIngressResource.ClusterIngress)

	return &clusterIngress, nil
}

var _ clients.ResourceClient = &ResourceClient{}

func (rc *ResourceClient) Kind() string {
	return resources.Kind(&v1alpha1.ClusterIngress{})
}

func (rc *ResourceClient) NewResource() resources.Resource {
	return resources.Clone(&v1alpha1.ClusterIngress{})
}

func (rc *ResourceClient) Register() error {
	return nil
}

func (rc *ResourceClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	contextutils.LoggerFrom(context.Background()).DPanic("this client does not support read operations")
	return nil, fmt.Errorf("this client does not support read operations")
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	contextutils.LoggerFrom(context.Background()).DPanic("this client does not support write operations")
	return nil, fmt.Errorf("this client does not support write operations")
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	contextutils.LoggerFrom(context.Background()).DPanic("this client does not support delete operations")
	return fmt.Errorf("this client does not support delete operations")
}

func (rc *ResourceClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	contextutils.LoggerFrom(context.Background()).DPanic("this client does not support apply status operations")
	return nil, fmt.Errorf("this client does not support apply status operations")
}

func (rc *ResourceClient) List(_ string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()

	clusterIngressObjList, err := rc.cache.ClusterIngressLister().List(labels.SelectorFromSet(opts.Selector))
	if err != nil {
		return nil, errors.Wrapf(err, "listing ClusterIngresses")
	}
	var resourceList resources.ResourceList
	for _, ClusterIngressObj := range clusterIngressObjList {
		resource := FromKube(ClusterIngressObj)

		if resource == nil {
			continue
		}
		resourceList = append(resourceList, resource)
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().GetName() < resourceList[j].GetMetadata().GetName()
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(_ string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	opts = opts.WithDefaults()
	watch := rc.cache.Subscribe()

	resourcesChan := make(chan resources.ResourceList)
	errs := make(chan error)
	// prevent flooding the channel with duplicates
	var previous *resources.ResourceList
	updateResourceList := func() {
		list, err := rc.List("", clients.ListOpts{
			Ctx:      opts.Ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		if previous != nil {
			if list.Equal(*previous) {
				return
			}
		}
		previous = &list
		resourcesChan <- list
	}

	go func() {
		defer rc.cache.Unsubscribe(watch)
		defer close(resourcesChan)
		defer close(errs)

		// watch should open up with an initial read
		updateResourceList()
		for {
			select {
			case _, ok := <-watch:
				if !ok {
					return
				}
				updateResourceList()
			case <-opts.Ctx.Done():
				return
			}
		}
	}()

	return resourcesChan, errs, nil
}

func (rc *ResourceClient) exist(ctx context.Context, namespace, name string) bool {
	_, err := rc.knativeClient.NetworkingV1alpha1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

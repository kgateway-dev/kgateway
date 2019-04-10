package clusteringress

import (
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/gogo/protobuf/types"
	knativev1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	knativeclientset "github.com/knative/serving/pkg/client/clientset/versioned"
	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubewatch "k8s.io/apimachinery/pkg/watch"
)

const typeUrl = "networking.internal.knative.dev/v1alpha1/ClusterIngress"

type ResourceClient struct {
	knative      knativeclientset.Interface
	ownerLabel   string
	resourceName string
	resourceType resources.Resource
}

func NewResourceClient(kube knativeclientset.Interface, resourceType resources.Resource) *ResourceClient {
	return &ResourceClient{
		knative:      kube,
		resourceName: reflect.TypeOf(resourceType).String(),
		resourceType: resourceType,
	}
}

func FromKube(ingress *knativev1alpha1.ClusterIngress) (*v1.ClusterIngress, error) {
	rawSpec, err := json.Marshal(ingress.Spec)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling knative ingress object")
	}
	spec := &types.Any{
		TypeUrl: typeUrl,
		Value:   rawSpec,
	}

	rawStatus, err := json.Marshal(ingress.Status)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling knative ingress object")
	}
	status := &types.Any{
		TypeUrl: typeUrl,
		Value:   rawStatus,
	}

	resource := &v1.ClusterIngress{
		ClusterIngressSpec:   spec,
		ClusterIngressStatus: status,
	}

	resource.SetMetadata(kubeutils.FromKubeMeta(ingress.ObjectMeta))

	return resource, nil
}

func ToKube(resource resources.Resource) (*knativev1alpha1.ClusterIngress, error) {
	ingResource, ok := resource.(*v1.ClusterIngress)
	if !ok {
		return nil, errors.Errorf("internal error: invalid resource %v passed to ingress-only client", resources.Kind(resource))
	}
	if ingResource.ClusterIngressSpec == nil {
		return nil, errors.Errorf("internal error: %v ingress spec cannot be nil", ingResource.GetMetadata().Ref())
	}
	var ingress knativev1alpha1.ClusterIngress
	if err := json.Unmarshal(ingResource.ClusterIngressSpec.Value, &ingress.Spec); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling knative ingress spec data")
	}
	if ingResource.ClusterIngressStatus != nil {
		if err := json.Unmarshal(ingResource.ClusterIngressStatus.Value, &ingress.Status); err != nil {
			return nil, errors.Wrapf(err, "unmarshalling knative ingress status data")
		}
	}

	meta := kubeutils.ToKubeMeta(resource.GetMetadata())
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	ingress.ObjectMeta = meta
	return &ingress, nil
}

var _ clients.ResourceClient = &ResourceClient{}

func (rc *ResourceClient) Kind() string {
	return resources.Kind(rc.resourceType)
}

func (rc *ResourceClient) NewResource() resources.Resource {
	return resources.Clone(rc.resourceType)
}

func (rc *ResourceClient) Register() error {
	return nil
}

func (rc *ResourceClient) Read(namespace, name string, opts clients.ReadOpts) (resources.Resource, error) {
	opts.Ctx = contextutils.WithLogger(opts.Ctx, "clusteringress-resource-client")
	logger := contextutils.LoggerFrom(opts.Ctx)
	if namespace != "" {
		logger.Warnf("cluster ingresses are cluster-scoped, namespace %v will be ignored", namespace)
		namespace = ""
	}
	if err := resources.ValidateName(name); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	opts = opts.WithDefaults()

	ingressObj, err := rc.knative.NetworkingV1alpha1().ClusterIngresses().Get(name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewNotExistErr(namespace, name, err)
		}
		return nil, errors.Wrapf(err, "reading ingressObj from kubernetes")
	}
	resource, err := FromKube(ingressObj)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, errors.Errorf("ingressObj %v is not kind %v", name, rc.Kind())
	}
	return resource, nil
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()

	opts.Ctx = contextutils.WithLogger(opts.Ctx, "clusteringress-resource-client")
	logger := contextutils.LoggerFrom(opts.Ctx)
	if meta.Namespace != "" {
		logger.Warnf("cluster ingresses are cluster-scoped, namespace %v will be ignored", meta.Namespace)
		meta.Namespace = ""
	}

	// mutate and return clone
	clone := resources.Clone(resource)
	clone.SetMetadata(meta)
	ingressObj, err := ToKube(resource)
	if err != nil {
		return nil, err
	}

	original, err := rc.Read(meta.Namespace, meta.Name, clients.ReadOpts{
		Ctx: opts.Ctx,
	})
	if original != nil && err == nil {
		if !opts.OverwriteExisting {
			return nil, errors.NewExistErr(meta)
		}
		if meta.ResourceVersion != original.GetMetadata().ResourceVersion {
			return nil, errors.NewResourceVersionErr(meta.Namespace, meta.Name, meta.ResourceVersion, original.GetMetadata().ResourceVersion)
		}
		if _, err := rc.knative.NetworkingV1alpha1().ClusterIngresses().Update(ingressObj); err != nil {
			return nil, errors.Wrapf(err, "updating knative ingressObj %v", ingressObj.Name)
		}
	} else {
		if _, err := rc.knative.NetworkingV1alpha1().ClusterIngresses().Create(ingressObj); err != nil {
			return nil, errors.Wrapf(err, "creating knative ingressObj %v", ingressObj.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(ingressObj.Namespace, ingressObj.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts.Ctx = contextutils.WithLogger(opts.Ctx, "clusteringress-resource-client")
	logger := contextutils.LoggerFrom(opts.Ctx)
	if namespace != "" {
		logger.Warnf("cluster ingresses are cluster-scoped, namespace %v will be ignored", namespace)
	}

	opts = opts.WithDefaults()
	if !rc.exist(name) {
		if !opts.IgnoreNotExist {
			return errors.NewNotExistErr("", name)
		}
		return nil
	}

	if err := rc.knative.NetworkingV1alpha1().ClusterIngresses().Delete(name, nil); err != nil {
		return errors.Wrapf(err, "deleting ingressObj %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()
	opts.Ctx = contextutils.WithLogger(opts.Ctx, "clusteringress-resource-client")
	logger := contextutils.LoggerFrom(opts.Ctx)
	if namespace != "" {
		logger.Warnf("cluster ingresses are cluster-scoped, namespace %v will be ignored", namespace)
	}

	ingressObjList, err := rc.knative.NetworkingV1alpha1().ClusterIngresses().List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(opts.Selector).String(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "listing ingressObjs at cluster level")
	}
	var resourceList resources.ResourceList
	for _, ingressObj := range ingressObjList.Items {
		resource, err := FromKube(&ingressObj)
		if err != nil {
			return nil, err
		}
		if resource == nil {
			continue
		}
		resourceList = append(resourceList, resource)
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().Name < resourceList[j].GetMetadata().Name
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	opts = opts.WithDefaults()
	opts.Ctx = contextutils.WithLogger(opts.Ctx, "clusteringress-resource-client")
	logger := contextutils.LoggerFrom(opts.Ctx)
	if namespace != "" {
		logger.Warnf("cluster ingresses are cluster-scoped, namespace %v will be ignored", namespace)
		namespace = ""
	}

	watch, err := rc.knative.NetworkingV1alpha1().ClusterIngresses().Watch(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(opts.Selector).String(),
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "initiating knative watch at cluster level")
	}
	resourcesChan := make(chan resources.ResourceList)
	errs := make(chan error)
	updateResourceList := func() {
		list, err := rc.List(namespace, clients.ListOpts{
			Ctx:      opts.Ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		resourcesChan <- list
	}

	go func() {
		// watch should open up with an initial read
		updateResourceList()
		for {
			select {
			case <-time.After(opts.RefreshRate):
				updateResourceList()
			case event := <-watch.ResultChan():
				switch event.Type {
				case kubewatch.Error:
					errs <- errors.Errorf("error during watch: %v", event)
				default:
					updateResourceList()
				}
			case <-opts.Ctx.Done():
				watch.Stop()
				close(resourcesChan)
				close(errs)
				return
			}
		}
	}()

	return resourcesChan, errs, nil
}

func (rc *ResourceClient) exist(name string) bool {
	_, err := rc.knative.NetworkingV1alpha1().ClusterIngresses().Get(name, metav1.GetOptions{})
	return err == nil
}

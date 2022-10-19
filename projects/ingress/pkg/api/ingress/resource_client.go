package ingress

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/golang/protobuf/ptypes/any"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubewatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const typeUrl = "k8s.io/networking.v1/Ingress"

type ResourceClient struct {
	kube         kubernetes.Interface
	ownerLabel   string
	resourceName string
	resourceType resources.Resource
}

func NewResourceClient(kube kubernetes.Interface, resourceType resources.Resource) *ResourceClient {
	return &ResourceClient{
		kube:         kube,
		resourceName: reflect.TypeOf(resourceType).String(),
		resourceType: resourceType,
	}
}

func FromKube(ingress *networkingv1.Ingress) (*v1.Ingress, error) {
	rawSpec, err := json.Marshal(ingress.Spec)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling kube ingress object")
	}
	spec := &any.Any{
		TypeUrl: typeUrl,
		Value:   rawSpec,
	}

	rawStatus, err := json.Marshal(ingress.Status)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling kube ingress object")
	}
	status := &any.Any{
		TypeUrl: typeUrl,
		Value:   rawStatus,
	}

	resource := &v1.Ingress{
		KubeIngressSpec:   spec,
		KubeIngressStatus: status,
	}

	resource.SetMetadata(kubeutils.FromKubeMeta(ingress.ObjectMeta, true))

	return resource, nil
}

func ToKube(resource resources.Resource) (*networkingv1.Ingress, error) {
	ingResource, ok := resource.(*v1.Ingress)
	if !ok {
		return nil, errors.Errorf("internal error: invalid resource %v passed to ingress-only client", resources.Kind(resource))
	}
	if ingResource.GetKubeIngressSpec() == nil {
		return nil, errors.Errorf("internal error: %v ingress spec cannot be nil", ingResource.GetMetadata().Ref())
	}
	var ingress networkingv1.Ingress
	if err := json.Unmarshal(ingResource.GetKubeIngressSpec().GetValue(), &ingress.Spec); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling kube ingress spec data")
	}
	if ingResource.GetKubeIngressStatus() != nil {
		if err := json.Unmarshal(ingResource.GetKubeIngressStatus().GetValue(), &ingress.Status); err != nil {
			return nil, errors.Wrapf(err, "unmarshalling kube ingress status data")
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
	if err := resources.ValidateName(name); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	opts = opts.WithDefaults()
	namespace = clients.DefaultNamespaceIfEmpty(namespace)

	ingressObj, err := rc.kube.NetworkingV1().Ingresses(namespace).Get(opts.Ctx, name, metav1.GetOptions{})
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
	updated, err := rc.write(resource, opts)
	if err != nil {
		return nil, err
	}
	// workaround for setting ingress status
	clone := resources.Clone(resource)
	clone.SetMetadata(updated.GetMetadata())
	return rc.writeStatus(clone, opts)
}

func (rc *ResourceClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	wopts := clients.WriteOpts{}
	wopts = wopts.WithDefaults()
	wopts.Ctx = opts.Ctx
	return rc.writeStatus(inputResource, wopts)
}

func (rc *ResourceClient) write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.GetNamespace())

	// mutate and return clone
	clone := resources.Clone(resource)
	clone.SetMetadata(meta)
	ingressObj, err := ToKube(resource)
	if err != nil {
		return nil, err
	}

	original, err := rc.Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{
		Ctx: opts.Ctx,
	})
	if original != nil && err == nil {
		if !opts.OverwriteExisting {
			return nil, errors.NewExistErr(meta)
		}
		if meta.GetResourceVersion() != original.GetMetadata().GetResourceVersion() {
			return nil, errors.NewResourceVersionErr(meta.GetNamespace(), meta.GetName(), meta.GetResourceVersion(), original.GetMetadata().GetResourceVersion())
		}
		if _, err := rc.kube.NetworkingV1().Ingresses(ingressObj.Namespace).Update(opts.Ctx, ingressObj, metav1.UpdateOptions{}); err != nil {
			return nil, errors.Wrapf(err, "updating kube ingressObj %v", ingressObj.Name)
		}
	} else {
		if _, err := rc.kube.NetworkingV1().Ingresses(ingressObj.Namespace).Create(opts.Ctx, ingressObj, metav1.CreateOptions{}); err != nil {
			return nil, errors.Wrapf(err, "creating kube ingressObj %v", ingressObj.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(ingressObj.Namespace, ingressObj.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) writeStatus(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.GetNamespace())

	// mutate and return clone
	clone := resources.Clone(resource)
	clone.SetMetadata(meta)
	ingressObj, err := ToKube(resource)
	if err != nil {
		return nil, err
	}

	original, err := rc.Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{
		Ctx: opts.Ctx,
	})
	if original != nil && err == nil {
		if !opts.OverwriteExisting {
			return nil, errors.NewExistErr(meta)
		}
		if meta.GetResourceVersion() != original.GetMetadata().GetResourceVersion() {
			return nil, errors.NewResourceVersionErr(meta.GetNamespace(), meta.GetName(), meta.GetResourceVersion(), original.GetMetadata().GetResourceVersion())
		}
		if _, err := rc.kube.NetworkingV1().Ingresses(ingressObj.Namespace).UpdateStatus(opts.Ctx, ingressObj, metav1.UpdateOptions{}); err != nil {
			return nil, errors.Wrapf(err, "updating kube ingressObj status %v", ingressObj.Name)
		}
	} else {
		if _, err := rc.kube.NetworkingV1().Ingresses(ingressObj.Namespace).Create(opts.Ctx, ingressObj, metav1.CreateOptions{}); err != nil {
			return nil, errors.Wrapf(err, "creating kube ingressObj status %v", ingressObj.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(ingressObj.Namespace, ingressObj.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts = opts.WithDefaults()
	if !rc.exist(opts.Ctx, namespace, name) {
		if !opts.IgnoreNotExist {
			return errors.NewNotExistErr(namespace, name)
		}
		return nil
	}

	if err := rc.kube.NetworkingV1().Ingresses(namespace).Delete(opts.Ctx, name, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "deleting ingressObj %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()

	ingressObjList, err := rc.kube.NetworkingV1().Ingresses(namespace).List(opts.Ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(opts.Selector).String(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "listing ingressObjs in %v", namespace)
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
		return resourceList[i].GetMetadata().GetName() < resourceList[j].GetMetadata().GetName()
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	opts = opts.WithDefaults()
	watch, err := rc.kube.NetworkingV1().Ingresses(namespace).Watch(opts.Ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(opts.Selector).String(),
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "initiating kube watch in %v", namespace)
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

func (rc *ResourceClient) exist(ctx context.Context, namespace, name string) bool {
	_, err := rc.kube.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

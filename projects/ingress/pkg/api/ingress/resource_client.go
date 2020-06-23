package ingress

import (
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubewatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const typeUrl = "k8s.io/extensions.v1beta1/Ingress"

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

func FromKube(ingress *v1beta1.Ingress) (*v1.Ingress, error) {
	rawSpec, err := json.Marshal(ingress.Spec)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling kube ingress object")
	}
	spec := &types.Any{
		TypeUrl: typeUrl,
		Value:   rawSpec,
	}

	rawStatus, err := json.Marshal(ingress.Status)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling kube ingress object")
	}
	status := &types.Any{
		TypeUrl: typeUrl,
		Value:   rawStatus,
	}

	resource := &v1.Ingress{
		KubeIngressSpec:   spec,
		KubeIngressStatus: status,
	}

	resource.SetMetadata(kubeutils.FromKubeMeta(ingress.ObjectMeta))

	return resource, nil
}

func ToKube(resource resources.Resource) (*v1beta1.Ingress, error) {
	ingResource, ok := resource.(*v1.Ingress)
	if !ok {
		return nil, errors.Errorf("internal error: invalid resource %v passed to ingress-only client", resources.Kind(resource))
	}
	if ingResource.KubeIngressSpec == nil {
		return nil, errors.Errorf("internal error: %v ingress spec cannot be nil", ingResource.GetMetadata().Ref())
	}
	var ingress v1beta1.Ingress
	if err := json.Unmarshal(ingResource.KubeIngressSpec.Value, &ingress.Spec); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling kube ingress spec data")
	}
	if ingResource.KubeIngressStatus != nil {
		if err := json.Unmarshal(ingResource.KubeIngressStatus.Value, &ingress.Status); err != nil {
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

	ingressObj, err := rc.kube.ExtensionsV1beta1().Ingresses(namespace).Get(name, metav1.GetOptions{})
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

func (rc *ResourceClient) write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.Namespace)

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
		if _, err := rc.kube.ExtensionsV1beta1().Ingresses(ingressObj.Namespace).Update(ingressObj); err != nil {
			return nil, errors.Wrapf(err, "updating kube ingressObj %v", ingressObj.Name)
		}
	} else {
		if _, err := rc.kube.ExtensionsV1beta1().Ingresses(ingressObj.Namespace).Create(ingressObj); err != nil {
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
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.Namespace)

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
		if _, err := rc.kube.ExtensionsV1beta1().Ingresses(ingressObj.Namespace).UpdateStatus(ingressObj); err != nil {
			return nil, errors.Wrapf(err, "updating kube ingressObj status %v", ingressObj.Name)
		}
	} else {
		if _, err := rc.kube.ExtensionsV1beta1().Ingresses(ingressObj.Namespace).Create(ingressObj); err != nil {
			return nil, errors.Wrapf(err, "creating kube ingressObj status %v", ingressObj.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(ingressObj.Namespace, ingressObj.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts = opts.WithDefaults()
	if !rc.exist(namespace, name) {
		if !opts.IgnoreNotExist {
			return errors.NewNotExistErr(namespace, name)
		}
		return nil
	}

	if err := rc.kube.ExtensionsV1beta1().Ingresses(namespace).Delete(name, nil); err != nil {
		return errors.Wrapf(err, "deleting ingressObj %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()

	ingressObjList, err := rc.kube.ExtensionsV1beta1().Ingresses(namespace).List(metav1.ListOptions{
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
		return resourceList[i].GetMetadata().Name < resourceList[j].GetMetadata().Name
	})

	return resourceList, nil
}

func (rc *ResourceClient) Watch(namespace string, opts clients.WatchOpts) (<-chan resources.ResourceList, <-chan error, error) {
	opts = opts.WithDefaults()
	watch, err := rc.kube.ExtensionsV1beta1().Ingresses(namespace).Watch(metav1.ListOptions{
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

func (rc *ResourceClient) exist(namespace, name string) bool {
	_, err := rc.kube.ExtensionsV1beta1().Ingresses(namespace).Get(name, metav1.GetOptions{})
	return err == nil
}

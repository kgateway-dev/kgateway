package service

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/golang/protobuf/ptypes/any"
	v1 "github.com/solo-io/gloo/projects/ingress/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/shared"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	kubewatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const typeUrl = "k8s.io/core.v1/Service"

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

func FromKube(svc *kubev1.Service) (*v1.KubeService, error) {
	rawSpec, err := json.Marshal(svc.Spec)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling kube svc object")
	}
	spec := &any.Any{
		TypeUrl: typeUrl,
		Value:   rawSpec,
	}

	rawStatus, err := json.Marshal(svc.Status)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling kube svc object")
	}
	status := &any.Any{
		TypeUrl: typeUrl,
		Value:   rawStatus,
	}

	resource := &v1.KubeService{
		KubeServiceSpec:   spec,
		KubeServiceStatus: status,
	}

	resource.SetMetadata(kubeutils.FromKubeMeta(svc.ObjectMeta, true))

	return resource, nil
}

func ToKube(resource resources.Resource) (*kubev1.Service, error) {
	ingResource, ok := resource.(*v1.KubeService)
	if !ok {
		return nil, errors.Errorf("internal error: invalid resource %v passed to svc-only client", resources.Kind(resource))
	}
	if ingResource.GetKubeServiceSpec() == nil {
		return nil, errors.Errorf("internal error: %v svc spec cannot be nil", ingResource.GetMetadata().Ref())
	}
	var svc kubev1.Service
	if err := json.Unmarshal(ingResource.GetKubeServiceSpec().GetValue(), &svc.Spec); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling kube svc spec data")
	}
	if ingResource.GetKubeServiceStatus() != nil {
		if err := json.Unmarshal(ingResource.GetKubeServiceStatus().GetValue(), &svc.Status); err != nil {
			return nil, errors.Wrapf(err, "unmarshalling kube svc status data")
		}
	}

	meta := kubeutils.ToKubeMeta(resource.GetMetadata())
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	svc.ObjectMeta = meta
	return &svc, nil
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

	svcObj, err := rc.kube.CoreV1().Services(namespace).Get(opts.Ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewNotExistErr(namespace, name, err)
		}
		return nil, errors.Wrapf(err, "reading svcObj from kubernetes")
	}
	resource, err := FromKube(svcObj)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, errors.Errorf("svcObj %v is not kind %v", name, rc.Kind())
	}
	return resource, nil
}

func (rc *ResourceClient) Write(resource resources.Resource, opts clients.WriteOpts) (resources.Resource, error) {
	opts = opts.WithDefaults()
	if err := resources.Validate(resource); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	meta := resource.GetMetadata()
	meta.Namespace = clients.DefaultNamespaceIfEmpty(meta.GetNamespace())

	// mutate and return clone
	clone := resources.Clone(resource)
	clone.SetMetadata(meta)
	svcObj, err := ToKube(resource)
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
		if _, err := rc.kube.CoreV1().Services(svcObj.Namespace).Update(opts.Ctx, svcObj, metav1.UpdateOptions{}); err != nil {
			return nil, errors.Wrapf(err, "updating kube svcObj %v", svcObj.Name)
		}
	} else {
		if _, err := rc.kube.CoreV1().Services(svcObj.Namespace).Create(opts.Ctx, svcObj, metav1.CreateOptions{}); err != nil {
			return nil, errors.Wrapf(err, "creating kube svcObj %v", svcObj.Name)
		}
	}

	// return a read object to update the resource version
	return rc.Read(svcObj.Namespace, svcObj.Name, clients.ReadOpts{Ctx: opts.Ctx})
}

func (rc *ResourceClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (resources.Resource, error) {
	name := inputResource.GetMetadata().GetName()
	namespace := inputResource.GetMetadata().GetNamespace()
	if err := resources.ValidateName(name); err != nil {
		return nil, errors.Wrapf(err, "validation error")
	}
	opts = opts.WithDefaults()

	data, err := shared.GetJsonPatchData(opts.Ctx, inputResource)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting status json patch data")
	}
	serviceObj, err := rc.kube.CoreV1().Services(namespace).Patch(opts.Ctx, name, types.JSONPatchType, data, metav1.PatchOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewNotExistErr(namespace, name, err)
		}
		return nil, errors.Wrapf(err, "patching serviceObj from kubernetes")
	}
	resource, err := FromKube(serviceObj)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return nil, errors.Errorf("serviceObj %v is not kind %v", name, rc.Kind())
	}
	return resource, nil
}

func (rc *ResourceClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	opts = opts.WithDefaults()
	if !rc.exist(opts.Ctx, namespace, name) {
		if !opts.IgnoreNotExist {
			return errors.NewNotExistErr(namespace, name)
		}
		return nil
	}

	if err := rc.kube.CoreV1().Services(namespace).Delete(opts.Ctx, name, metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "deleting svcObj %v", name)
	}
	return nil
}

func (rc *ResourceClient) List(namespace string, opts clients.ListOpts) (resources.ResourceList, error) {
	opts = opts.WithDefaults()

	svcObjList, err := rc.kube.CoreV1().Services(namespace).List(opts.Ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(opts.Selector).String(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "listing svcObjs in %v", namespace)
	}
	var resourceList resources.ResourceList
	for _, svcObj := range svcObjList.Items {
		resource, err := FromKube(&svcObj)
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
	watch, err := rc.kube.CoreV1().Services(namespace).Watch(opts.Ctx, metav1.ListOptions{
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
	_, err := rc.kube.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

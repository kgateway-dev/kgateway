package fake

import (
	"context"
	"fmt"

	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/test"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/pkg/consts"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ apiclient.Client = (*cli)(nil)

func init() {
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)

	// Register the legacy XListenerSet list kind so the fake dynamic client can
	// back the delayed legacy informer used by older Gateway API versions.
	kube.FakeIstioScheme.AddKnownTypeWithName(
		wellknown.XListenerSetGVK.GroupVersion().WithKind("XListenerSetList"),
		&unstructured.UnstructuredList{},
	)
}

type cli struct {
	kube.Client
	kgateway versioned.Interface
}

func NewClient(t test.Failer, objects ...client.Object) *cli {
	return NewClientWithExtraGVRs(t, nil, objects...)
}

func NewClientWithExtraGVRs(t test.Failer, extraGVRs []schema.GroupVersionResource, objects ...client.Object) *cli {
	known, kgw := filterObjects(objects...)
	c := &cli{
		Client:   fakeIstioClient(known...),
		kgateway: fakeKgwClient(kgw...),
	}

	allCRDs := append(testutils.AllCRDs, extraGVRs...)
	for _, crd := range allCRDs {
		clienttest.MakeCRDWithAnnotations(t, c.Client, crd, map[string]string{
			consts.BundleVersionAnnotation: consts.BundleVersion,
		})
	}
	seedCRDs(t, c.Client, allCRDs)

	apiclient.RegisterTypes()

	return c
}

func (c *cli) Kgateway() versioned.Interface {
	return c.kgateway
}

func (c *cli) Core() kube.Client {
	return c.Client
}

func fakeIstioClient(objects ...client.Object) kube.Client {
	c := kube.NewFakeClient(testutils.ToRuntimeObjects(objects...)...)
	// Also add to the Dynamic store
	for _, obj := range objects {
		nn := kubeutils.NamespacedNameFrom(obj)
		gvr := mustGetGVR(obj, kube.IstioScheme)
		d := c.Dynamic().Resource(gvr).Namespace(obj.GetNamespace())
		us, err := kubeutils.ToUnstructured(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to convert to unstructured for object %T %s: %v", obj, nn, err))
		}
		_, err = d.Create(context.Background(), us, metav1.CreateOptions{})
		if err != nil {
			panic(fmt.Sprintf("failed to create in dynamic client for object %T %s: %v", obj, nn, err))
		}
	}

	return c
}

func fakeKgwClient(objects ...client.Object) *fake.Clientset {
	// The generated clientset in this repo does not include the newer NewClientset helper
	// because we do not generate applyconfigs for these APIs yet.
	//nolint:staticcheck // SA1019: use the generated fake until applyconfig generation is enabled
	f := fake.NewSimpleClientset()
	for _, obj := range objects {
		gvr := mustGetGVR(obj, schemes.DefaultScheme())
		// Run Create() instead of Add(), so we can pass the GVR. Otherwise, Kubernetes guesses, and it guesses wrong for 'GatewayParameters'.
		// DeepCopy since it will mutate the managed fields/etc
		if err := f.Tracker().Create(gvr, obj.DeepCopyObject(), obj.(metav1.ObjectMetaAccessor).GetObjectMeta().GetNamespace()); err != nil {
			panic("failed to create: " + err.Error())
		}
	}
	return f
}

func filterObjects(objects ...client.Object) (istio []client.Object, kgw []client.Object) {
	for _, obj := range objects {
		switch obj.(type) {
		case *kgateway.Backend,
			*kgateway.BackendConfigPolicy,
			*kgateway.DirectResponse,
			*kgateway.GatewayExtension,
			*kgateway.GatewayParameters,
			*kgateway.HTTPListenerPolicy,
			*kgateway.ListenerPolicy,
			*kgateway.TrafficPolicy:
			kgw = append(kgw, obj)
		default:
			istio = append(istio, obj)
		}
	}
	return istio, kgw
}

func mustGetGVR(obj client.Object, scheme *runtime.Scheme) schema.GroupVersionResource {
	gvr, err := getGVR(obj, scheme)
	if err != nil {
		panic(err)
	}
	return gvr
}

func getGVR(obj client.Object, scheme *runtime.Scheme) (schema.GroupVersionResource, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Group == "" {
		gvks, _, _ := scheme.ObjectKinds(obj)
		gvk = gvks[0]
	}
	gvr, err := wellknown.GVKToGVR(gvk)
	if err != nil {
		// try unsafe guess
		gvr, _ = meta.UnsafeGuessKindToResource(gvk)
		if gvr == (schema.GroupVersionResource{}) {
			return schema.GroupVersionResource{}, fmt.Errorf("failed to get GVR for object %s: %v", kubeutils.NamespacedNameFrom(obj), err)
		}
	}
	if gvr.Group == "core" {
		gvr.Group = ""
	}
	return gvr, nil
}

func seedCRDs(t test.Failer, c kube.Client, gvrs []schema.GroupVersionResource) {
	t.Helper()

	crds := map[string]*apiextensionsv1.CustomResourceDefinition{}
	for _, gvr := range gvrs {
		name := fmt.Sprintf("%s.%s", gvr.Resource, gvr.Group)
		crd := crds[name]
		if crd == nil {
			crd = &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
					Annotations: map[string]string{
						consts.BundleVersionAnnotation: consts.BundleVersion,
					},
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: gvr.Group,
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Plural: gvr.Resource,
						Kind:   kindForSeededCRD(gvr),
					},
					Scope: apiextensionsv1.NamespaceScoped,
				},
			}
			crds[name] = crd
		}

		foundVersion := false
		for _, version := range crd.Spec.Versions {
			if version.Name == gvr.Version {
				foundVersion = true
				break
			}
		}
		if foundVersion {
			continue
		}

		crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
			Name:    gvr.Version,
			Served:  true,
			Storage: len(crd.Spec.Versions) == 0,
		})
	}

	for _, crd := range crds {
		extClient, ok := c.Ext().(*extfake.Clientset)
		if !ok {
			t.Fatal("unexpected apiextensions fake client type")
		}

		err := extClient.Tracker().Add(crd)
		if apierrors.IsAlreadyExists(err) {
			err = extClient.Tracker().Update(gvr.CustomResourceDefinition, crd, "")
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func kindForSeededCRD(resource schema.GroupVersionResource) string {
	switch resource {
	case gvr.KubernetesGateway:
		return wellknown.GatewayKind
	case gvr.GatewayClass:
		return wellknown.GatewayClassKind
	case gvr.HTTPRoute:
		return wellknown.HTTPRouteKind
	case gvr.GRPCRoute:
		return wellknown.GRPCRouteKind
	case gvr.TCPRoute:
		return wellknown.TCPRouteKind
	case gvr.TLSRoute, wellknown.TLSRouteV1Alpha3GVR:
		return wellknown.TLSRouteKind
	case gvr.ReferenceGrant:
		return wellknown.ReferenceGrantKind
	case gvr.BackendTLSPolicy, wellknown.BackendTLSPolicyGVR:
		return wellknown.BackendTLSPolicyKind
	case wellknown.XListenerSetGVR:
		return wellknown.XListenerSetKind
	case wellknown.ListenerSetGVR:
		return wellknown.ListenerSetKind
	case gvr.Service:
		return wellknown.ServiceKind
	case gvr.Pod:
		return "Pod"
	case gvr.ServiceEntry:
		return "ServiceEntry"
	case gvr.WorkloadEntry:
		return "WorkloadEntry"
	case gvr.AuthorizationPolicy:
		return "AuthorizationPolicy"
	case wellknown.BackendGVR:
		return wellknown.BackendGVK.Kind
	case wellknown.BackendConfigPolicyGVR:
		return wellknown.BackendConfigPolicyGVK.Kind
	case wellknown.TrafficPolicyGVR:
		return wellknown.TrafficPolicyGVK.Kind
	case wellknown.HTTPListenerPolicyGVR:
		return wellknown.HTTPListenerPolicyGVK.Kind
	case wellknown.ListenerPolicyGVR:
		return wellknown.ListenerPolicyGVK.Kind
	case wellknown.DirectResponseGVR:
		return wellknown.DirectResponseGVK.Kind
	case wellknown.GatewayExtensionGVR:
		return wellknown.GatewayExtensionGVK.Kind
	case wellknown.GatewayParametersGVR:
		return wellknown.GatewayParametersGVK.Kind
	default:
		return resource.Resource
	}
}

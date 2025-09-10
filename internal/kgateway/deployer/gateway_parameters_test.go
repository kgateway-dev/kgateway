package deployer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	api "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	gw2_v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
)

const (
	defaultNamespace = "default"
)

type testHelmValuesGenerator struct{}

func (thv *testHelmValuesGenerator) IsSelfManaged(ctx context.Context, gw client.Object) (bool, error) {
	return true, nil
}

func (thv *testHelmValuesGenerator) GetValues(ctx context.Context, gw client.Object) (map[string]any, error) {
	return map[string]any{
		"testHelmValuesGenerator": struct{}{},
	}, nil
}

func TestIsSelfManagedOnGatewayClass(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()
	gwParams.Spec.SelfManaged = &gw2_v1alpha1.SelfManagedGateway{}

	gw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io",
		},
		Spec: api.GatewaySpec{
			GatewayClassName: wellknown.DefaultGatewayClassName,
		},
	}

	gwp := NewGatewayParameters(newFakeClientWithObjs(gwc, gwParams), defaultInputs(t, gwc, gw))
	selfManaged, err := gwp.IsSelfManaged(context.Background(), gw)
	assert.NoError(t, err)
	assert.True(t, selfManaged)
}

func TestIsSelfManagedOnGateway(t *testing.T) {
	gwc := defaultGatewayClass()
	defaultGwp := emptyGatewayParameters()

	// gateway params attached to gateway
	customGwp := emptyGatewayParameters()
	customGwp.ObjectMeta.Name = "custom-gwp"
	customGwp.ObjectMeta.Namespace = defaultNamespace
	customGwp.Spec.SelfManaged = &gw2_v1alpha1.SelfManagedGateway{}

	gw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io",
		},
		Spec: api.GatewaySpec{
			GatewayClassName: wellknown.DefaultGatewayClassName,
			Infrastructure: &api.GatewayInfrastructure{
				ParametersRef: &api.LocalParametersReference{
					Group: gw2_v1alpha1.GroupName,
					Kind:  api.Kind(wellknown.GatewayParametersGVK.Kind),
					Name:  "custom-gwp",
				},
			},
		},
	}

	gwp := NewGatewayParameters(newFakeClientWithObjs(gwc, defaultGwp, customGwp), defaultInputs(t, gwc, gw))
	selfManaged, err := gwp.IsSelfManaged(context.Background(), gw)
	assert.NoError(t, err)
	assert.True(t, selfManaged)
}

func TestIsSelfManagedWithExtendedGatewayParameters(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()
	extraGwParams := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: defaultNamespace},
	}

	gw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io",
		},
		Spec: api.GatewaySpec{
			Infrastructure: &api.GatewayInfrastructure{
				ParametersRef: &api.LocalParametersReference{
					Group: "v1",
					Kind:  "ConfigMap",
					Name:  "testing",
				},
			},
			GatewayClassName: wellknown.DefaultGatewayClassName,
		},
	}

	gwp := NewGatewayParameters(newFakeClientWithObjs(gwc, gwParams, extraGwParams), defaultInputs(t, gwc, gw)).
		WithExtraGatewayParameters(deployer.ExtraGatewayParameters{Group: "v1", Kind: "ConfigMap", Object: extraGwParams, Generator: &testHelmValuesGenerator{}})
	selfManaged, err := gwp.IsSelfManaged(context.Background(), gw)
	assert.NoError(t, err)
	assert.True(t, selfManaged)
}

func TestShouldUseDefaultGatewayParameters(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()

	gw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io",
		},
		Spec: api.GatewaySpec{
			GatewayClassName: wellknown.DefaultGatewayClassName,
		},
	}

	gwp := NewGatewayParameters(newFakeClientWithObjs(gwc, gwParams), defaultInputs(t, gwc, gw))
	vals, err := gwp.GetValues(context.Background(), gw)

	assert.NoError(t, err)
	assert.Contains(t, vals, "gateway")

	selfManaged, err := gwp.IsSelfManaged(context.Background(), gw)
	assert.NoError(t, err)
	assert.False(t, selfManaged)
}

func TestShouldUseExtendedGatewayParameters(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()
	extraGwParams := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: defaultNamespace},
	}

	gw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io",
		},
		Spec: api.GatewaySpec{
			Infrastructure: &api.GatewayInfrastructure{
				ParametersRef: &api.LocalParametersReference{
					Group: "v1",
					Kind:  "ConfigMap",
					Name:  "testing",
				},
			},
			GatewayClassName: wellknown.DefaultGatewayClassName,
		},
	}

	gwp := NewGatewayParameters(newFakeClientWithObjs(gwc, gwParams, extraGwParams), defaultInputs(t, gwc, gw)).
		WithExtraGatewayParameters(deployer.ExtraGatewayParameters{Group: "v1", Kind: "ConfigMap", Object: extraGwParams, Generator: &testHelmValuesGenerator{}})
	vals, err := gwp.GetValues(context.Background(), gw)

	assert.NoError(t, err)
	assert.Contains(t, vals, "testHelmValuesGenerator")
}

func TestGatewayGVKsToWatch(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()
	cli := newFakeClientWithObjs(gwc, gwParams)
	gwp := NewGatewayParameters(cli, defaultInputs(t, gwc))

	d, err := NewGatewayDeployer(wellknown.DefaultGatewayControllerName, cli, gwp)
	assert.NoError(t, err)

	gvks, err := GatewayGVKsToWatch(context.TODO(), d)
	assert.NoError(t, err)
	assert.Len(t, gvks, 4)
	assert.ElementsMatch(t, gvks, []schema.GroupVersionKind{
		wellknown.DeploymentGVK,
		wellknown.ServiceGVK,
		wellknown.ServiceAccountGVK,
		wellknown.ConfigMapGVK,
	})
}

func TestInferencePoolGVKsToWatch(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()
	cli := newFakeClientWithObjs(gwc, gwParams)

	d, err := NewInferencePoolDeployer(wellknown.DefaultGatewayControllerName, cli)
	assert.NoError(t, err)

	gvks, err := InferencePoolGVKsToWatch(context.TODO(), d)
	assert.NoError(t, err)
	assert.Len(t, gvks, 4)
	assert.ElementsMatch(t, gvks, []schema.GroupVersionKind{
		wellknown.DeploymentGVK,
		wellknown.ServiceGVK,
		wellknown.ServiceAccountGVK,
		wellknown.ClusterRoleBindingGVK,
	})
}

func defaultGatewayClass() *api.GatewayClass {
	return &api.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: wellknown.DefaultGatewayClassName,
		},
		Spec: api.GatewayClassSpec{
			ControllerName: wellknown.DefaultGatewayControllerName,
			ParametersRef: &api.ParametersReference{
				Group:     gw2_v1alpha1.GroupName,
				Kind:      api.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      wellknown.DefaultGatewayParametersName,
				Namespace: ptr.To(api.Namespace(defaultNamespace)),
			},
		},
	}
}

func emptyGatewayParameters() *gw2_v1alpha1.GatewayParameters {
	return &gw2_v1alpha1.GatewayParameters{
		TypeMeta: metav1.TypeMeta{
			Kind: wellknown.GatewayParametersGVK.Kind,
			// The parsing expects GROUP/VERSION format in this field
			APIVersion: gw2_v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      wellknown.DefaultGatewayParametersName,
			Namespace: defaultNamespace,
			UID:       "1237",
		},
	}
}

func defaultInputs(t *testing.T, objs ...client.Object) *deployer.Inputs {
	return &deployer.Inputs{
		CommonCollections: newCommonCols(t, objs...),
		Dev:               false,
		ControlPlane: deployer.ControlPlaneInfo{
			XdsHost: "something.cluster.local",
			XdsPort: 1234,
		},
		ImageInfo: &deployer.ImageInfo{
			Registry: "foo",
			Tag:      "bar",
		},
		GatewayClassName:         wellknown.DefaultGatewayClassName,
		WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
		AgentGatewayClassName:    wellknown.DefaultAgentGatewayClassName,
	}
}

// initialize a fake controller-runtime client with the given list of objects
func newFakeClientWithObjs(objs ...client.Object) client.Client {
	scheme := schemes.GatewayScheme()

	// Ensure the rbac types are registered.
	if err := rbacv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add rbacv1 scheme: %v", err))
	}

	// Check if any object is an InferencePool, and add its scheme if needed.
	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk.Kind == wellknown.InferencePoolKind {
			if err := inf.Install(scheme); err != nil {
				panic(fmt.Sprintf("failed to add InferenceExtension scheme: %v", err))
			}
			break
		}
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
}

func newCommonCols(t test.Failer, initObjs ...client.Object) *common.CommonCollections {
	ctx := context.Background()
	var anys []any
	for _, obj := range initObjs {
		anys = append(anys, obj)
	}
	mock := krttest.NewMock(t, anys)

	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, extensionsplug.ContributesPolicies{}, settings.Settings{})
	kubeRawGateways := krttest.GetMockCollection[*api.Gateway](mock)
	kubeRawListenerSets := krttest.GetMockCollection[*apixv1a1.XListenerSet](mock)
	gatewayClasses := krttest.GetMockCollection[*api.GatewayClass](mock)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(ctx, krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	gateways := krtcollections.NewGatewayIndex(krtopts, wellknown.DefaultGatewayControllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, nsCol)

	commonCols := &common.CommonCollections{
		GatewayIndex: gateways,
	}

	for !kubeRawGateways.HasSynced() || !kubeRawListenerSets.HasSynced() || !gatewayClasses.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	gateways.Gateways.WaitUntilSynced(ctx.Done())
	return commonCols
}

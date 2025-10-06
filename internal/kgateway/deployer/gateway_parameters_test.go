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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	api "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	gw2_v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
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
			Listeners: []api.Listener{
				{
					Protocol: api.HTTPProtocolType,
					Port:     80,
					Name:     "http",
				},
			},
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

	d, err := NewGatewayDeployer(wellknown.DefaultGatewayControllerName, wellknown.DefaultAgwControllerName, wellknown.DefaultAgwClassName, cli, gwp)
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

	d, err := NewInferencePoolDeployer(wellknown.DefaultGatewayControllerName, wellknown.DefaultAgwControllerName, wellknown.DefaultAgwClassName, cli)
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

func TestAgentgatewayAndEnvoyContainerDistinctValues(t *testing.T) {
	// Create GatewayParameters with agentgateway disabled and distinct values
	gwParams := &gw2_v1alpha1.GatewayParameters{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.GatewayParametersGVK.Kind,
			APIVersion: gw2_v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-disabled-params",
			Namespace: "default",
		},
		Spec: gw2_v1alpha1.GatewayParametersSpec{
			Kube: &gw2_v1alpha1.KubernetesProxyConfig{
				Agentgateway: &gw2_v1alpha1.Agentgateway{
					Enabled: ptr.To(false), // Explicitly disabled
					Image: &gw2_v1alpha1.Image{
						Registry:   ptr.To("agent-registry"),
						Repository: ptr.To("agent-repo"),
						Tag:        ptr.To("agent-tag"),
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsUser: ptr.To(int64(12345)),
					},
					Env: []corev1.EnvVar{
						{
							Name:  "AGENT_ENV",
							Value: "agent-value",
						},
					},
				},
				EnvoyContainer: &gw2_v1alpha1.EnvoyContainer{
					Image: &gw2_v1alpha1.Image{
						Registry:   ptr.To("envoy-registry"),
						Repository: ptr.To("envoy-repo"),
						Tag:        ptr.To("envoy-tag"),
						PullPolicy: ptr.To(corev1.PullNever),
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsUser: ptr.To(int64(54321)),
					},
					Env: []corev1.EnvVar{
						{
							Name:  "ENVOY_ENV",
							Value: "envoy-value",
						},
					},
					Bootstrap: &gw2_v1alpha1.EnvoyBootstrap{
						LogLevel: ptr.To("info"),
					},
				},
			},
		},
	}

	gwc := &api.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-disabled-gwc",
		},
		Spec: api.GatewayClassSpec{
			ControllerName: wellknown.DefaultGatewayControllerName,
			ParametersRef: &api.ParametersReference{
				Group:     gw2_v1alpha1.GroupName,
				Kind:      api.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      "agent-disabled-params",
				Namespace: ptr.To(api.Namespace("default")),
			},
		},
	}

	gw := &api.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway-disabled",
			Namespace: "default",
			UID:       "test-disabled",
		},
		Spec: api.GatewaySpec{
			GatewayClassName: "agent-disabled-gwc",
			Listeners: []api.Listener{{
				Name: "listener-1",
				Port: 80,
			}},
		},
	}

	gwp := NewGatewayParameters(newFakeClientWithObjs(gwc, gwParams), defaultInputs(t, gwc, gw))
	vals, err := gwp.GetValues(context.Background(), gw)
	assert.NoError(t, err)

	gateway, ok := vals["gateway"].(map[string]any)
	assert.True(t, ok, "gateway should be present in helm values")

	// Verify that envoyContainerConfig values are used (not agentgateway values)
	// Check image values
	image, ok := gateway["image"].(map[string]any)
	assert.True(t, ok, "image should be present")
	assert.Equal(t, "envoy-registry", image["registry"])
	assert.Equal(t, "envoy-repo", image["repository"])
	assert.Equal(t, "envoy-tag", image["tag"])
	assert.Equal(t, "Never", image["pullPolicy"])

	// Check resources
	resources, ok := gateway["resources"].(map[string]any)
	assert.True(t, ok, "resources should be present")
	requests, ok := resources["requests"].(map[string]any)
	assert.True(t, ok, "requests should be present")
	assert.Equal(t, "100m", requests["cpu"])
	assert.Equal(t, "128Mi", requests["memory"])

	// Check security context
	securityContext, ok := gateway["securityContext"].(map[string]any)
	assert.True(t, ok, "securityContext should be present")
	runAsUser, ok := securityContext["runAsUser"]
	assert.True(t, ok, "runAsUser should be present")
	assert.Equal(t, float64(54321), runAsUser)

	// Check environment variables
	env, ok := gateway["env"].([]any)
	assert.True(t, ok, "env should be present")
	assert.Len(t, env, 1)
	envVar, ok := env[0].(map[string]any)
	assert.True(t, ok, "env var should be a map")
	assert.Equal(t, "ENVOY_ENV", envVar["name"])
	assert.Equal(t, "envoy-value", envVar["value"])
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
			XdsHost:    "something.cluster.local",
			XdsPort:    1234,
			AgwXdsPort: 5678,
		},
		ImageInfo: &deployer.ImageInfo{
			Registry: "foo",
			Tag:      "bar",
		},
		GatewayClassName:         wellknown.DefaultGatewayClassName,
		WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
		AgentgatewayClassName:    wellknown.DefaultAgwClassName,
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

func newCommonCols(t test.Failer, initObjs ...client.Object) *collections.CommonCollections {
	ctx := context.Background()
	var anys []any
	for _, obj := range initObjs {
		anys = append(anys, obj)
	}
	mock := krttest.NewMock(t, anys)

	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, apisettings.Settings{})
	kubeRawGateways := krttest.GetMockCollection[*api.Gateway](mock)
	kubeRawListenerSets := krttest.GetMockCollection[*apixv1a1.XListenerSet](mock)
	gatewayClasses := krttest.GetMockCollection[*api.GatewayClass](mock)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(ctx, krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	gateways := krtcollections.NewGatewayIndex(krtopts, wellknown.DefaultGatewayControllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, nsCol)

	commonCols := &collections.CommonCollections{
		GatewayIndex: gateways,
	}

	for !kubeRawGateways.HasSynced() || !kubeRawListenerSets.HasSynced() || !gatewayClasses.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	gateways.Gateways.WaitUntilSynced(ctx.Done())
	return commonCols
}

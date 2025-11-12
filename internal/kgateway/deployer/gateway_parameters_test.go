package deployer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/util/smallset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	gw2_v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

const (
	defaultNamespace = "default"
)

type testHelmValuesGenerator struct{}

func (thv *testHelmValuesGenerator) GetValues(ctx context.Context, gw client.Object) (map[string]any, error) {
	return map[string]any{
		"testHelmValuesGenerator": struct{}{},
	}, nil
}

func (thv *testHelmValuesGenerator) GetCacheSyncHandlers() []cache.InformerSynced {
	return nil
}

func TestShouldUseDefaultGatewayParameters(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: wellknown.DefaultGatewayClassName,
			Listeners: []gwv1.Listener{
				{
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
					Name:     "http",
				},
			},
		},
	}

	ctx := t.Context()
	fakeClient := fake.NewClient(t, gwc, gwParams)
	gwp := NewGatewayParameters(fakeClient, defaultInputs(t, gwc, gw))
	fakeClient.RunAndWait(ctx.Done())
	vals, err := gwp.GetValues(ctx, gw)

	assert.NoError(t, err)
	assert.Contains(t, vals, "gateway")
}

func TestShouldUseExtendedGatewayParameters(t *testing.T) {
	gwc := defaultGatewayClass()
	gwParams := emptyGatewayParameters()
	extraGwParams := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: defaultNamespace},
	}

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: defaultNamespace,
			UID:       "1235",
		},
		Spec: gwv1.GatewaySpec{
			Infrastructure: &gwv1.GatewayInfrastructure{
				ParametersRef: &gwv1.LocalParametersReference{
					Group: "v1",
					Kind:  "ConfigMap",
					Name:  "testing",
				},
			},
			GatewayClassName: wellknown.DefaultGatewayClassName,
		},
	}

	ctx := t.Context()
	fakeClient := fake.NewClient(t, gwc, gwParams, extraGwParams)
	gwp := NewGatewayParameters(fakeClient, defaultInputs(t, gwc, gw)).
		WithHelmValuesGeneratorOverride(&testHelmValuesGenerator{})
	fakeClient.RunAndWait(ctx.Done())
	vals, err := gwp.GetValues(ctx, gw)

	assert.NoError(t, err)
	assert.Contains(t, vals, "testHelmValuesGenerator")
}

func TestAgentgatewayAndEnvoyContainerDistinctValues(t *testing.T) {
	// Create GatewayParameters with agentgateway disabled and distinct values
	gwParams := &gw2_v1alpha1.GatewayParameters{
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

	gwc := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-disabled-gwc",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: wellknown.DefaultGatewayControllerName,
			ParametersRef: &gwv1.ParametersReference{
				Group:     gw2_v1alpha1.GroupName,
				Kind:      gwv1.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      "agent-disabled-params",
				Namespace: ptr.To(gwv1.Namespace("default")),
			},
		},
	}

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway-disabled",
			Namespace: "default",
			UID:       "test-disabled",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "agent-disabled-gwc",
			Listeners: []gwv1.Listener{{
				Name: "listener-1",
				Port: 80,
			}},
		},
	}

	ctx := t.Context()
	fakeClient := fake.NewClient(t, gwc, gwParams)
	gwp := NewGatewayParameters(fakeClient, defaultInputs(t, gwc, gw))
	fakeClient.RunAndWait(ctx.Done())
	vals, err := gwp.GetValues(ctx, gw)
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

func defaultGatewayClass() *gwv1.GatewayClass {
	return &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: wellknown.DefaultGatewayClassName,
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: wellknown.DefaultGatewayControllerName,
			ParametersRef: &gwv1.ParametersReference{
				Group:     gw2_v1alpha1.GroupName,
				Kind:      gwv1.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      wellknown.DefaultGatewayParametersName,
				Namespace: ptr.To(gwv1.Namespace(defaultNamespace)),
			},
		},
	}
}

func emptyGatewayParameters() *gw2_v1alpha1.GatewayParameters {
	return &gw2_v1alpha1.GatewayParameters{
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
		GatewayClassName:           wellknown.DefaultGatewayClassName,
		WaypointGatewayClassName:   wellknown.DefaultWaypointClassName,
		AgentgatewayClassName:      wellknown.DefaultAgwClassName,
		AgentgatewayControllerName: wellknown.DefaultAgwControllerName,
	}
}

func newCommonCols(t test.Failer, initObjs ...client.Object) *collections.CommonCollections {
	ctx := context.Background()
	var anys []any
	for _, obj := range initObjs {
		anys = append(anys, obj)
	}
	mock := krttest.NewMock(t, anys)

	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, apisettings.Settings{})
	kubeRawGateways := krttest.GetMockCollection[*gwv1.Gateway](mock)
	kubeRawListenerSets := krttest.GetMockCollection[*apixv1a1.XListenerSet](mock)
	gatewayClasses := krttest.GetMockCollection[*gwv1.GatewayClass](mock)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(ctx, krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	gateways := krtcollections.NewGatewayIndex(krtopts, smallset.New(wellknown.DefaultGatewayControllerName), wellknown.DefaultGatewayControllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, nsCol)

	commonCols := &collections.CommonCollections{
		GatewayIndex: gateways,
	}

	for !kubeRawGateways.HasSynced() || !kubeRawListenerSets.HasSynced() || !gatewayClasses.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	gateways.Gateways.WaitUntilSynced(ctx.Done())
	return commonCols
}

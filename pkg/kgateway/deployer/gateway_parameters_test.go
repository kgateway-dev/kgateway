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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
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

func defaultGatewayClass() *gwv1.GatewayClass {
	return &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: wellknown.DefaultGatewayClassName,
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: wellknown.DefaultGatewayControllerName,
			ParametersRef: &gwv1.ParametersReference{
				Group:     kgateway.GroupName,
				Kind:      gwv1.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      wellknown.DefaultGatewayParametersName,
				Namespace: ptr.To(gwv1.Namespace(defaultNamespace)),
			},
		},
	}
}

func emptyGatewayParameters() *kgateway.GatewayParameters {
	return &kgateway.GatewayParameters{
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

	settings := apisettings.Settings{
		EnableEnvoy:        true,
		EnableAgentgateway: true,
	}

	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, settings)
	kubeRawGateways := krttest.GetMockCollection[*gwv1.Gateway](mock)
	kubeRawListenerSets := krttest.GetMockCollection[*apixv1a1.XListenerSet](mock)
	gatewayClasses := krttest.GetMockCollection[*gwv1.GatewayClass](mock)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(ctx, krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	gatewayIndexConfig := krtcollections.GatewayIndexConfig{
		KrtOpts:             krtopts,
		ControllerNames:     smallset.New(wellknown.DefaultGatewayControllerName),
		EnvoyControllerName: wellknown.DefaultGatewayControllerName,
		PolicyIndex:         policies,
		Gateways:            kubeRawGateways,
		ListenerSets:        kubeRawListenerSets,
		GatewayClasses:      gatewayClasses,
		Namespaces:          nsCol,
	}
	gateways := krtcollections.NewGatewayIndex(gatewayIndexConfig)
	commonCols := &collections.CommonCollections{
		GatewayIndex: gateways,
		Settings:     settings,
	}

	for !kubeRawGateways.HasSynced() || !kubeRawListenerSets.HasSynced() || !gatewayClasses.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	gateways.Gateways.WaitUntilSynced(ctx.Done())
	return commonCols
}

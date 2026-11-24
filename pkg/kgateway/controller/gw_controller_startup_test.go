package controller

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/util/smallset"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/test/envtestassets"
)

// gatedSyncer reports synced only once released, standing in for a krt collection that has not
// computed its rows yet.
type gatedSyncer chan struct{}

func (g gatedSyncer) HasSynced() bool {
	select {
	case <-g:
		return true
	default:
		return false
	}
}

func (g gatedSyncer) WaitUntilSynced(stop <-chan struct{}) bool {
	select {
	case <-g:
		return true
	case <-stop:
		return false
	}
}

// TestGatewayReconcilerWaitsForGatewaysForDeployer restarts the Gateway controller against a
// live proxy Service that already exposes a ListenerSet's port. Until GatewaysForDeployer syncs,
// the deployer can only guess the ports from the Gateway's own listeners, so a reconcile in that
// window would apply a Service without the ListenerSet's port and remove it from the live
// Service.
func TestGatewayReconcilerWaitsForGatewaysForDeployer(t *testing.T) {
	const (
		className = "startup-cls"
		ns        = "default"
		gwName    = "startup-gw"
	)

	assetsDir, err := envtestassets.GetEnvTestAssetsDir()
	require.NoError(t, err)
	env := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway-crds", "templates"),
		},
		ErrorIfCRDPathMissing:   true,
		BinaryAssetsDirectory:   assetsDir,
		ControlPlaneStopTimeout: 5 * time.Second,
	}
	restCfg, err := env.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Logf("error stopping envtest: %v", err)
		}
	})

	scheme := schemes.GatewayScheme()
	cli, err := client.New(restCfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, cli.Create(ctx, &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: className},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gatewayControllerName},
	}))
	require.NoError(t, cli.Create(ctx, &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: gwName, Namespace: ns},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: className,
			AllowedListeners: &gwv1.AllowedListeners{
				Namespaces: &gwv1.ListenerNamespaces{From: new(gwv1.NamespacesFromSame)},
			},
			Listeners: []gwv1.Listener{{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}},
		},
	}))
	createListenerSet := func(name string, port gwv1.PortNumber) {
		t.Helper()
		require.NoError(t, cli.Create(ctx, &gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: gwv1.ListenerSetSpec{
				ParentRef: gwv1.ParentGatewayReference{Name: gwName},
				Listeners: []gwv1.ListenerEntry{{Name: "http", Port: port, Protocol: gwv1.HTTPProtocolType}},
			},
		}))
	}
	createListenerSet("ls-8080", 8080)

	servicePorts := func() []int32 {
		svc := &corev1.Service{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: gwName}, svc); err != nil {
			return nil
		}
		var ports []int32
		for _, p := range svc.Spec.Ports {
			ports = append(ports, p.Port)
		}
		slices.Sort(ports)
		return ports
	}

	// The first controller computes the ListenerSet's port and puts it on the Service.
	stopFirst := startGatewayControllerForTest(t, restCfg, scheme, nil)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, []int32{80, 8080}, servicePorts())
	}, defaultPollTimeout, 100*time.Millisecond, "the Service should expose the Gateway's and the ListenerSet's ports")
	stopFirst()

	// While no controller runs, attach another ListenerSet. Its port shows up on the Service only
	// once the restarted controller reconciles from GatewaysForDeployer.
	createListenerSet("ls-9090", 9090)

	// Restart with GatewaysForDeployer not yet synced.
	gate := make(gatedSyncer)
	rows := krt.NewMutableCollection[ir.GatewayForDeployer](gate, nil)
	startGatewayControllerForTest(t, restCfg, scheme, func(cc *collections.CommonCollections) {
		cc.GatewayIndex.GatewaysForDeployer = rows.AsCollection()
	})

	assert.Never(t, func() bool {
		return !slices.Equal(servicePorts(), []int32{80, 8080})
	}, 3*time.Second, 50*time.Millisecond,
		"the Service must keep the ListenerSet's port while GatewaysForDeployer has not synced")

	rows.UpdateObject(ir.GatewayForDeployer{
		ObjectSource: ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      wellknown.GatewayKind,
			Namespace: ns,
			Name:      gwName,
		},
		ControllerName: gatewayControllerName,
		Ports:          smallset.New[int32](80, 8080, 9090),
	})
	close(gate)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, []int32{80, 8080, 9090}, servicePorts())
	}, defaultPollTimeout, 100*time.Millisecond, "once synced, the controller should apply the ports from GatewaysForDeployer")
}

// startGatewayControllerForTest runs the Gateway and GatewayClass controllers until the returned
// function is called or the test ends. mutate, if set, can replace collections before the
// controllers are built.
func startGatewayControllerForTest(
	t *testing.T,
	restCfg *rest.Config,
	scheme *runtime.Scheme,
	mutate func(*collections.CommonCollections),
) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	kubeClient, err := apiclient.New(restCfg)
	require.NoError(t, err)
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:     scheme,
		Controller: config.Controller{SkipNameValidation: new(true)},
		Metrics:    metricsserver.Options{BindAddress: "0"},
	})
	require.NoError(t, err)

	commonCols, err := newCommonCols(ctx, kubeClient)
	require.NoError(t, err)
	if mutate != nil {
		mutate(commonCols)
	}

	require.NoError(t, NewBaseGatewayController(ctx, GatewayConfig{
		Client:         kubeClient,
		Mgr:            mgr,
		ControllerName: gatewayControllerName,
		ImageInfo: &deployer.ImageInfo{
			Registry: "ghcr.io/kgateway-dev",
			Tag:      "latest",
		},
		DiscoveryNamespaceFilter: fakeDiscoveryNamespaceFilter{},
		CommonCollections:        commonCols,
	}, map[string]*deployer.GatewayClassInfo{}, nil, nil))
	kubeClient.RunAndWait(ctx.Done())

	mgrDone := make(chan error, 1)
	go func() {
		mgrDone <- mgr.Start(ctx)
	}()

	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case err := <-mgrDone:
			assert.NoError(t, err, "controller manager returned an error")
		case <-time.After(defaultPollTimeout):
			t.Error("timed out waiting for the controller manager to stop")
		}
		kubeClient.Shutdown()
	}
	t.Cleanup(stop)
	return stop
}

package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	istiosets "istio.io/istio/pkg/util/sets"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/test/envtestassets"
)

const (
	gatewayClassName            = "clsname"
	altGatewayClassName         = "clsname-alt"
	selfManagedGatewayClassName = "clsname-selfmanaged"
	gatewayControllerName       = "kgateway.dev/kgateway"
	defaultNamespace            = "default"

	localhost = "127.0.0.1"
)

// gwClasses maps the default GatewayClasses initialized in startController
var (
	gwClasses           = []string{gatewayClassName, altGatewayClassName, selfManagedGatewayClassName}
	gwClassToController = map[string]string{
		gatewayClassName:            gatewayControllerName,
		altGatewayClassName:         gatewayControllerName,
		selfManagedGatewayClassName: gatewayControllerName,
	}
	// defaultPollTimeout is the default timeout for polling operations
	defaultPollTimeout = 10 * time.Second
)

type ControllerSuite struct {
	suite.Suite

	// fields below are set in SetupSuite
	suitCtxCancelFn context.CancelFunc
	env             *envtest.Environment
	client          client.Client //nolint:forbidigo // can use client.Client in envtest
	kubeconfigPath  string
	managerErr      chan error
	paramsStatus    *reports.GatewayParametersStatus
}

func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerSuite))
}

func (s *ControllerSuite) SetupSuite() {
	// Don't use the testing.T.Context because it is cancelled before the corresponding
	// Cleanup function is called, and we need the Client/Manager to be alive in t.Cleanup handlers
	ctx, cancel := context.WithCancel(context.Background())
	s.suitCtxCancelFn = cancel

	// Create a scheme and add Gateway types.
	scheme := schemes.GatewayScheme()

	assetsDir, err := envtestassets.GetEnvTestAssetsDir()
	s.Require().NoError(err)

	s.env = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway-crds", "templates"),
		},
		ErrorIfCRDPathMissing: true,
		// set assets dir so we can run without the makefile
		BinaryAssetsDirectory:   assetsDir,
		ControlPlaneStopTimeout: 5 * time.Second,
	}

	controllerLogger := logr.FromSlogHandler(slog.Default().Handler())
	ctrl.SetLogger(controllerLogger)
	cfg, err := s.env.Start()
	s.Require().NoError(err)
	s.Require().NotNil(cfg)

	s.client, err = client.New(cfg, client.Options{Scheme: scheme})
	s.Require().NoError(err)
	s.Require().NotNil(s.client)

	err = s.startController(ctx, cfg, scheme, s.env)
	s.Require().NoError(err)
}

// Does not use s.Require() so that we can perform all cleanup steps without early termination
func (s *ControllerSuite) TearDownSuite() {
	// Envtest must be stopped after the manager/controllers stop, so cancel the Context first
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571#issuecomment-945535598
	s.suitCtxCancelFn()
	if s.managerErr != nil {
		select {
		case err := <-s.managerErr:
			assert.NoError(s.T(), err, "controller-manager returned error")
		case <-time.After(defaultPollTimeout):
			s.T().Error("timed out waiting for controller-manager to stop")
		}
	}
	err := s.env.Stop()
	if err != nil {
		s.T().Logf("error stopping Envtest after manager exit %v", err)
	}

	err = os.Remove(s.kubeconfigPath)
	s.NoError(err)
}

// TestGatewayStatus tests the Status on Gateway creation
func (s *ControllerSuite) TestGatewayStatus() {
	testCases := []struct {
		name         string
		gatewayClass string
	}{
		{
			name:         "default gateway class",
			gatewayClass: gatewayClassName,
		},
		{
			name:         "alternate gateway class",
			gatewayClass: altGatewayClassName,
		},
		{
			name:         "self-managed gateway class",
			gatewayClass: selfManagedGatewayClassName,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			t := s.T()
			r := require.New(t)
			ctx := t.Context()
			gwName := "test-" + tc.gatewayClass
			gwNamespace := "default"

			t.Cleanup(func() {
				gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: gwName, Namespace: gwNamespace}}
				err := s.client.Delete(context.Background(), gw)
				if err != nil && k8serrors.IsNotFound(err) {
					return
				}
				r.NoError(err, "error deleting Gateway")
			})

			gw := gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: gwNamespace,
				},
				Spec: gwv1.GatewaySpec{
					Addresses: []gwv1.GatewaySpecAddress{{
						Type:  new(gwv1.IPAddressType),
						Value: localhost,
					}},
					GatewayClassName: gwv1.ObjectName(tc.gatewayClass),
					Listeners: []gwv1.Listener{{
						Protocol: "HTTP",
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: new(gwv1.NamespacesFromSame),
							},
						},
						Name: "listener",
					}},
				},
			}
			err := s.client.Create(context.Background(), &gw)
			r.NoError(err, "error creating Gateway")

			if tc.gatewayClass != selfManagedGatewayClassName {
				// Update the status of the service for the controller to pick up.
				// We use EventuallyWithT to ensure the status update succeeds on retry if there
				// is a conflict with the object written by the controller.
				r.EventuallyWithT(func(c *assert.CollectT) {
					cur := &corev1.Service{}
					err := s.client.Get(ctx, types.NamespacedName{Name: gwName, Namespace: gwNamespace}, cur)
					require.NoError(c, err, "error getting Gateway Service")

					cur.Status = corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{{IP: localhost}},
						},
					}

					err = s.client.Status().Patch(ctx, cur, client.Merge)
					require.NoError(c, err, "error updating Gateway Service status")
				}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for Gateway Service to be created")
			}
			r.EventuallyWithT(func(c *assert.CollectT) {
				var gotGw gwv1.Gateway
				err := s.client.Get(ctx, types.NamespacedName{Name: gwName, Namespace: gwNamespace}, &gotGw)
				require.NoError(c, err, "error getting Gateway")
				require.NotEmpty(c, gotGw.Status.Addresses, "expected Gateway to have status addresses")

				require.Len(c, gotGw.Status.Addresses, 1)
				require.Equal(c, gwv1.IPAddressType, *gotGw.Status.Addresses[0].Type)
				require.Equal(c, localhost, gotGw.Status.Addresses[0].Value)
			}, defaultPollTimeout, 500*time.Millisecond)
		})
	}
}

// TestInvalidGatewayParameters tests that a Gateway with invalid GatewayParameters attached
func (s *ControllerSuite) TestInvalidGatewayParameters() {
	ctx := context.Background()
	var gwp *kgateway.GatewayParameters
	var gw *gwv1.Gateway

	s.T().Cleanup(func() {
		err := s.client.Delete(ctx, gwp)
		s.NoError(err)
		err = s.client.Delete(ctx, gw)
		s.NoError(err)
	})

	gwp = &kgateway.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-gwp",
			Namespace: "default",
		},
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				Deployment: &kgateway.ProxyDeployment{
					Replicas: new(int32(2)),
				},
			},
		},
	}
	gw = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(gatewayClassName),
			Infrastructure: &gwv1.GatewayInfrastructure{
				ParametersRef: &gwv1.LocalParametersReference{
					Group: kgateway.GroupName,
					Kind:  "InvalidKindName",
					Name:  gwp.Name,
				},
			},
			Listeners: []gwv1.Listener{{
				Name:     "listener",
				Protocol: "HTTP",
				Port:     80,
			}},
		},
	}
	err := s.client.Create(ctx, gwp)
	s.Require().NoError(err)
	err = s.client.Create(ctx, gw)
	s.Require().NoError(err)

	// The status writer, which this suite does not run, reports the failed verdict as
	// Accepted=False/InvalidParameters.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		verdict := s.verdictFor(types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace})
		require.NotNil(c, verdict, "the deployer should publish its verdict")
		require.Contains(c, verdict.Err(), "InvalidKindName")
		require.Equal(c, int64(1), verdict.ObservedGeneration())
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the deployer's failed verdict")
}

// TestInvalidGatewayParametersClearedWithoutValidPorts tests that fixing the parametersRef
// removes the Gateway's parameters error even when the Gateway still has no valid ports.
func (s *ControllerSuite) TestInvalidGatewayParametersClearedWithoutValidPorts() {
	ctx := context.Background()
	gwp := &kgateway.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reserved-port-gwp",
			Namespace: "default",
		},
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{},
		},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reserved-port-gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(gatewayClassName),
			Infrastructure: &gwv1.GatewayInfrastructure{
				ParametersRef: &gwv1.LocalParametersReference{
					Group: kgateway.GroupName,
					Kind:  "InvalidKindName",
					Name:  gwp.Name,
				},
			},
			Listeners: []gwv1.Listener{{
				Name:     "listener",
				Protocol: "HTTP",
				// A reserved port: once the parameters resolve, there are no valid ports to deploy.
				Port: 9091,
			}},
		},
	}
	s.Require().NoError(s.client.Create(ctx, gwp))
	s.Require().NoError(s.client.Create(ctx, gw))
	s.T().Cleanup(func() {
		s.NoError(s.client.Delete(ctx, gw))
		s.NoError(s.client.Delete(ctx, gwp))
	})

	nn := types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		verdict := s.verdictFor(nn)
		require.NotNil(c, verdict, "the deployer should publish its verdict")
		require.NotEmpty(c, verdict.Err(), "the verdict should be a failure")
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the deployer's failed verdict")

	s.Require().NoError(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := s.client.Get(ctx, nn, gw); err != nil {
			return err
		}
		gw.Spec.Infrastructure.ParametersRef.Kind = gwv1.Kind(wellknown.GatewayParametersGVK.Kind)
		return s.client.Update(ctx, gw)
	}))

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		verdict := s.verdictFor(nn)
		require.NotNil(c, verdict, "the deployer should publish its verdict")
		require.Empty(c, verdict.Err(), "fixing the parametersRef should make the verdict a success")
		require.Equal(c, int64(2), verdict.ObservedGeneration(), "the verdict should be for the fixed generation")
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the deployer's successful verdict")
}

// verdictFor reads the deployer's verdict for gw from the status contributions it publishes.
func (s *ControllerSuite) verdictFor(gw types.NamespacedName) *reports.GatewayParametersReport {
	for _, contribution := range s.paramsStatus.Contributions().List() {
		if contribution.Target.NamespacedName == gw && contribution.GatewayParameters != nil {
			return contribution.GatewayParameters
		}
	}
	return nil
}

// TestBootstrapOverlayAppliedCondition tests that the Gateway reconciler reports the
// kgateway.dev/BootstrapOverlayApplied condition as the bootstrap overlays in an existing
// Gateway's GatewayParameters change.
func (s *ControllerSuite) TestBootstrapOverlayAppliedCondition() {
	key := types.NamespacedName{Name: "bootstrap-overlays", Namespace: defaultNamespace}

	// Create a Gateway whose GatewayParameters have a valid overlay.
	s.createGatewayWithOverlay(key, `{"stats_flush_interval": "10s"}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapOverlayApplied, false)

	// An overlay with a template is rejected. The proxy keeps running, so the Gateway
	// stays accepted, and its image did not change, so no image is named.
	s.updateOverlay(key, `{"layered_runtime": {"layers": [{"name": "{{ .Values.name }}", "static_layer": {}}]}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, false)
	s.NotContains(s.bootstrapOverlayAppliedMessage(key), "The proxy keeps running image")
	clientset, err := kubernetes.NewForConfig(s.env.Config)
	s.Require().NoError(err)
	evts, err := clientset.EventsV1().Events(key.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.False(slices.ContainsFunc(evts.Items, func(e eventsv1.Event) bool {
		return e.Regarding.Name == key.Name && e.Reason == "ProxyImageHeldBack"
	}), "no ProxyImageHeldBack event should be recorded when the image did not change")

	// Removing the overlays removes the condition.
	s.updateOverlay(key, "")
	s.waitForBootstrapOverlayApplied(key, "", "", false)
}

// TestBootstrapOverlaysInvalidOnNewGateway tests that a new Gateway whose bootstrap
// overlays cannot be applied gets no proxy and is not accepted until they are fixed.
func (s *ControllerSuite) TestBootstrapOverlaysInvalidOnNewGateway() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "bootstrap-overlays-new", Namespace: defaultNamespace}

	// Create a Gateway whose GatewayParameters have an invalid overlay.
	s.createGatewayWithOverlay(key, `{"layerd_runtime": {}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, true)
	err := s.client.Get(ctx, key, &appsv1.Deployment{})
	s.Require().True(k8serrors.IsNotFound(err), "no proxy should be deployed, got %v", err)

	// Fixing the overlay deploys the proxy and accepts the Gateway.
	s.updateOverlay(key, `{"layered_runtime": {}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapOverlayApplied, false)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		require.NoError(c, s.client.Get(ctx, key, &appsv1.Deployment{}))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the proxy Deployment")
}

// TestBootstrapOverlaysHeldBackImage tests that when an invalid overlay keeps an existing
// Gateway's Deployment, a proxy image change in the same edit is reported as held back.
func (s *ControllerSuite) TestBootstrapOverlaysHeldBackImage() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "bootstrap-held-back-image", Namespace: defaultNamespace}

	// Deploy a Gateway with valid overlays, and read its proxy image.
	s.createGatewayWithOverlay(key, `{"stats_flush_interval": "10s"}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapOverlayApplied, false)
	var running string
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		dep := &appsv1.Deployment{}
		require.NoError(c, s.client.Get(ctx, key, dep))
		require.NotEmpty(c, dep.Spec.Template.Spec.Containers)
		running = dep.Spec.Template.Spec.Containers[0].Image
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the proxy Deployment")

	// Record the versions of the proxy objects.
	dep, cm, svc := &appsv1.Deployment{}, &corev1.ConfigMap{}, &corev1.Service{}
	s.Require().NoError(s.client.Get(ctx, key, dep))
	s.Require().NoError(s.client.Get(ctx, key, cm))
	s.Require().NoError(s.client.Get(ctx, key, svc))

	// Change the proxy image and break the overlay in one edit.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gwp := &kgateway.GatewayParameters{}
		require.NoError(c, s.client.Get(ctx, key, gwp))
		setBootstrapOverlay(gwp, `{"layerd_runtime": {}}`)
		gwp.Spec.Kube.EnvoyContainer.Image = &kgateway.Image{Tag: new("held-back")}
		require.NoError(c, s.client.Update(ctx, gwp))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out updating GatewayParameters")

	// The condition and a Warning event name the running and the held-back image.
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, false)
	heldBack := "The proxy keeps running image " + running + "; image "
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		msg := s.bootstrapOverlayAppliedMessage(key)
		require.Contains(c, msg, heldBack)
		require.Contains(c, msg, ":held-back applies once the Gateway's objects are deployed.")
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the held-back image message")
	s.waitForGatewayEvent(key, "ProxyImageHeldBack", heldBack)

	// The invalid overlay leaves every proxy object as it was, so the Deployment keeps
	// the running image.
	for _, obj := range []client.Object{dep, cm, svc} {
		got := obj.DeepCopyObject().(client.Object)
		s.Require().NoError(s.client.Get(ctx, key, got))
		s.Equal(obj.GetResourceVersion(), got.GetResourceVersion(), "the invalid overlay should leave %T unchanged", obj)
	}
}

// bootstrapOverlayAppliedMessage returns the message of the Gateway's
// kgateway.dev/BootstrapOverlayApplied condition, or "" without one.
func (s *ControllerSuite) bootstrapOverlayAppliedMessage(key types.NamespacedName) string {
	gw := &gwv1.Gateway{}
	s.Require().NoError(s.client.Get(context.Background(), key, gw))
	if cond := meta.FindStatusCondition(gw.Status.Conditions, conditions.KgatewayConditionBootstrapOverlayApplied); cond != nil {
		return cond.Message
	}
	return ""
}

// TestBootstrapOverlayEvents tests that the Gateway reconciler records the events the
// deployer reports on the Gateway, for applied overlays and for overlays that fail.
func (s *ControllerSuite) TestBootstrapOverlayEvents() {
	key := types.NamespacedName{Name: "bootstrap-overlay-events", Namespace: defaultNamespace}

	// An overlay that sets a managed path records a Warning event that names it.
	s.createGatewayWithOverlay(key, `{"static_resources": {"clusters": [{"name": "xds_cluster", "connect_timeout": "9s"}]}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapManagedPathsOverridden, false)
	s.waitForGatewayEvent(key, conditions.KgatewayReasonBootstrapManagedPathsOverridden,
		"sets static_resources.clusters[name=xds_cluster] (GatewayParameters default/bootstrap-overlay-events)")

	// An overlay that fails records a Warning event with the error.
	s.updateOverlay(key, `{"layerd_runtime": {}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, false)
	s.waitForGatewayEvent(key, conditions.KgatewayReasonBootstrapInvalidOverlay, "Applied no bootstrap overlay")
}

// TestBootstrapOverlayEventsNotRepeated tests that a Gateway edit that leaves the
// kgateway.dev/BootstrapOverlayApplied condition as it was, other than its observed
// generation, does not record the overlay's Warning events again.
func (s *ControllerSuite) TestBootstrapOverlayEventsNotRepeated() {
	key := types.NamespacedName{Name: "bootstrap-overlay-events-once", Namespace: defaultNamespace}

	// An overlay that sets a managed path records a Warning event that names it.
	s.createGatewayWithOverlay(key, `{"static_resources": {"clusters": [{"name": "xds_cluster", "connect_timeout": "9s"}]}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapManagedPathsOverridden, false)
	s.waitForGatewayEvent(key, conditions.KgatewayReasonBootstrapManagedPathsOverridden, "sets static_resources.clusters[name=xds_cluster]")

	// Add a listener, and wait for the condition to observe the new generation.
	s.updateGateway(key, func(gw *gwv1.Gateway) {
		gw.Spec.Listeners = append(gw.Spec.Listeners, gwv1.Listener{Name: "second", Protocol: "HTTP", Port: 8081})
	})
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapManagedPathsOverridden, false)

	// The event was recorded once: there is one, and it is not a series.
	clientset, err := kubernetes.NewForConfig(s.env.Config)
	s.Require().NoError(err)
	evts, err := clientset.EventsV1().Events(key.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	var recorded []eventsv1.Event
	for _, e := range evts.Items {
		if e.Regarding.Name == key.Name && e.Reason == conditions.KgatewayReasonBootstrapManagedPathsOverridden {
			recorded = append(recorded, e)
		}
	}
	s.Require().Len(recorded, 1)
	s.Nil(recorded[0].Series, "the event should not have been recorded again")
}

// TestBootstrapOverlayConditionRemovedOnInvalidParameters tests that a Gateway whose
// bootstrap overlay applied loses the kgateway.dev/BootstrapOverlayApplied condition when
// its objects can no longer be rendered: nothing says whether the overlay would apply.
func (s *ControllerSuite) TestBootstrapOverlayConditionRemovedOnInvalidParameters() {
	key := types.NamespacedName{Name: "bootstrap-overlay-invalid-parameters", Namespace: defaultNamespace}

	// Create a Gateway whose GatewayParameters have a valid overlay.
	s.createGatewayWithOverlay(key, `{"stats_flush_interval": "10s"}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapOverlayApplied, false)

	// Point the Gateway at a GatewayParameters that does not exist. The Gateway is
	// rejected, and the overlay condition is removed.
	s.updateGateway(key, func(gw *gwv1.Gateway) { gw.Spec.Infrastructure.ParametersRef.Name = "missing" })
	s.waitForBootstrapOverlayApplied(key, "", "", true)
}

// TestBootstrapOverlayFailureReplacesInvalidParameters tests that when a deployed
// Gateway's parameters failed and now resolve with a bootstrap overlay that fails, both
// at one Gateway generation, the deployer's verdict stops being the earlier failure:
// the proxy keeps running, and only kgateway.dev/BootstrapOverlayApplied reports the overlay.
func (s *ControllerSuite) TestBootstrapOverlayFailureReplacesInvalidParameters() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "bootstrap-overlay-replaces-invalid", Namespace: defaultNamespace}

	// Deploy a Gateway whose GatewayParameters have a valid overlay.
	s.createGatewayWithOverlay(key, `{"stats_flush_interval": "10s"}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapOverlayApplied, false)
	gw := &gwv1.Gateway{}
	s.Require().NoError(s.client.Get(ctx, key, gw))
	generation := gw.Generation

	// Delete the GatewayParameters. The Gateway's parameters no longer resolve, so the
	// verdict is a failure.
	gwp := &kgateway.GatewayParameters{}
	s.Require().NoError(s.client.Get(ctx, key, gwp))
	s.Require().NoError(s.client.Delete(ctx, gwp))
	s.waitForBootstrapOverlayApplied(key, "", "", true)

	// Create the GatewayParameters again, with an overlay that fails.
	recreated := &kgateway.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
		Spec:       *gwp.Spec.DeepCopy(),
	}
	setBootstrapOverlay(recreated, `{"layerd_runtime": {}}`)
	s.Require().NoError(s.client.Create(ctx, recreated))

	// The overlay's failure is reported, and the verdict is no longer a failure.
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, false)

	// The Gateway was not edited, so the failure was replaced at the same generation.
	s.Require().NoError(s.client.Get(ctx, key, gw))
	s.Equal(generation, gw.Generation, "the Gateway should not have been edited")
}

// TestBootstrapOverlayConditionRemovedOnSelfManaged tests that a Gateway whose bootstrap
// overlay applied loses the kgateway.dev/BootstrapOverlayApplied condition when it switches
// to a self-managed GatewayParameters, for which kgateway deploys no proxy.
func (s *ControllerSuite) TestBootstrapOverlayConditionRemovedOnSelfManaged() {
	key := types.NamespacedName{Name: "bootstrap-overlay-self-managed", Namespace: defaultNamespace}

	// Create a Gateway whose GatewayParameters have a valid overlay.
	s.createGatewayWithOverlay(key, `{"stats_flush_interval": "10s"}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionTrue, conditions.KgatewayReasonBootstrapOverlayApplied, false)

	// Point the Gateway at the self-managed GatewayParameters. The overlay condition is
	// removed, and the Gateway is not rejected.
	s.updateGateway(key, func(gw *gwv1.Gateway) {
		gw.Spec.Infrastructure.ParametersRef.Name = selfManagedGatewayClassName
	})
	s.waitForBootstrapOverlayApplied(key, "", "", false)
}

// TestBootstrapOverlayLongError tests that a bootstrap overlay error longer than the API
// server accepts in a condition message or an event note is reported cut, so the status
// update and the event are not rejected.
func (s *ControllerSuite) TestBootstrapOverlayLongError() {
	key := types.NamespacedName{Name: "bootstrap-overlay-long-error", Namespace: defaultNamespace}

	// Create a Gateway whose overlay sets an unknown field, which the error names, with a
	// name longer than both limits.
	s.createGatewayWithOverlay(key, `{"`+strings.Repeat("x", 2*maxOverlayErrorLength)+`": {}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, true)

	// The condition message holds the start of the error and says it was cut.
	msg := s.bootstrapOverlayAppliedMessage(key)
	s.Len(msg, maxOverlayErrorLength)
	s.True(strings.HasSuffix(msg, truncatedSuffix), "the message should say it was cut, got %q", msg)

	// The event is recorded, with its note cut to the API server's limit.
	s.waitForGatewayEvent(key, conditions.KgatewayReasonBootstrapInvalidOverlay, truncatedSuffix)
	clientset, err := kubernetes.NewForConfig(s.env.Config)
	s.Require().NoError(err)
	evts, err := clientset.EventsV1().Events(key.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	for _, e := range evts.Items {
		if e.Regarding.Name == key.Name && e.Reason == conditions.KgatewayReasonBootstrapInvalidOverlay {
			s.Len(e.Note, maxEventNoteLength)
			s.True(strings.HasPrefix(e.Note, "Applied no bootstrap overlay: "), "the note should keep its start, got %q", e.Note)
		}
	}
}

// TestBootstrapOverlayNotMountedNote tests that when an overlay that fails could not
// have reached the proxy, the condition says so after the overlay's error, even when
// that error is cut.
func (s *ControllerSuite) TestBootstrapOverlayNotMountedNote() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "bootstrap-overlay-not-mounted-note", Namespace: defaultNamespace}

	// Create a Gateway whose overlay fails with an error longer than the limit.
	s.createGatewayWithOverlay(key, `{"`+strings.Repeat("x", 2*maxOverlayErrorLength)+`": {}}`)
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, true)
	s.NotContains(s.bootstrapOverlayAppliedMessage(key), "does not read the generated bootstrap",
		"the proxy would read the generated bootstrap, so there is no note")

	// Mount another ConfigMap at /etc/envoy.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gwp := &kgateway.GatewayParameters{}
		require.NoError(c, s.client.Get(ctx, key, gwp))
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`)}}
		require.NoError(c, s.client.Update(ctx, gwp))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out updating GatewayParameters")

	// The overlay's failure is still the reason, and the message ends with the note
	// after the cut error.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		msg := s.bootstrapOverlayAppliedMessage(key)
		require.Contains(c, msg, truncatedSuffix+" The proxy does not read the generated bootstrap (")
		require.True(c, strings.HasSuffix(msg, "remove the overlay, or fix it before removing that change, to deploy the Gateway's other changes."), "got %q", msg)
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the not-mounted note")
	s.waitForBootstrapOverlayApplied(key, metav1.ConditionFalse, conditions.KgatewayReasonBootstrapInvalidOverlay, true)
}

// TestBootstrapOverlayRequiresMergeStrategy tests that the API server rejects a
// GatewayParameters bootstrap overlay that leaves out mergeStrategy.
func (s *ControllerSuite) TestBootstrapOverlayRequiresMergeStrategy() {
	// Build a GatewayParameters whose overlay has a value and no mergeStrategy.
	gwp := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "bootstrap-overlay-no-strategy", "namespace": defaultNamespace},
		"spec": map[string]any{"kube": map[string]any{"envoyContainer": map[string]any{"bootstrap": map[string]any{
			"overlay": map[string]any{"value": map[string]any{"stats_flush_interval": "10s"}},
		}}}},
	}}
	gwp.SetGroupVersionKind(wellknown.GatewayParametersGVK)

	// Creating it fails validation and names the missing field.
	err := s.client.Create(context.Background(), gwp)
	s.Require().True(k8serrors.IsInvalid(err), "expected an Invalid error, got %v", err)
	s.ErrorContains(err, "spec.kube.envoyContainer.bootstrap.overlay.mergeStrategy: Required value")
}

// TestBootstrapConfigMapTooLargeWithoutOverlays tests that a Gateway without bootstrap
// overlays whose generated ConfigMap would exceed the API server's size limit keeps its
// proxy as it is, gets no BootstrapOverlayApplied condition, and gets a Warning event.
func (s *ControllerSuite) TestBootstrapConfigMapTooLargeWithoutOverlays() {
	ctx := context.Background()
	key := types.NamespacedName{Name: "bootstrap-too-large", Namespace: defaultNamespace}

	// Deploy a Gateway without bootstrap overlays, and read its proxy Deployment once
	// its generated ConfigMap holds the revision the Deployment runs.
	s.createGatewayWithOverlay(key, "")
	dep := &appsv1.Deployment{}
	var runningChecksum string
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		require.NoError(c, s.client.Get(ctx, key, dep))
		runningChecksum = dep.Spec.Template.Annotations[bootstrapoverlay.ChecksumAnnotation]
		require.NotEmpty(c, runningChecksum, "the proxy pod template should have a bootstrap checksum")
		cm := &corev1.ConfigMap{}
		require.NoError(c, s.client.Get(ctx, key, cm))
		require.Contains(c, cm.Data, "envoy-"+runningChecksum+".yaml", "the ConfigMap should hold the running revision")
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for the proxy Deployment and ConfigMap")

	// Create a ReplicaSet of the proxy Deployment that still uses an earlier revision.
	const earlierChecksum = "earlier"
	rsLabels := map[string]string{"app": key.Name + "-earlier"}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            key.Name + "-earlier",
			Namespace:       key.Namespace,
			Labels:          map[string]string{wellknown.GatewayNameLabel: key.Name},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dep, wellknown.DeploymentGVK)},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: new(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: rsLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      rsLabels,
					Annotations: map[string]string{bootstrapoverlay.ChecksumAnnotation: earlierChecksum},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "proxy", Image: "proxy"}}},
			},
		},
	}
	s.Require().NoError(s.client.Create(ctx, rs))
	s.T().Cleanup(func() { s.NoError(s.client.Delete(ctx, rs)) })

	// Add the earlier revision to the ConfigMap, sized so the ConfigMap holds exactly
	// as many bytes as the API server allows.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		cm := &corev1.ConfigMap{}
		require.NoError(c, s.client.Get(ctx, key, cm))
		earlierKey := "envoy-" + earlierChecksum + ".yaml"
		delete(cm.Data, earlierKey)
		size := 0
		for _, v := range cm.Data {
			size += len(v)
		}
		cm.Data[earlierKey] = strings.Repeat("#", 1024*1024-size)
		require.NoError(c, s.client.Update(ctx, cm))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out adding the earlier revision to the ConfigMap")

	// Set maxDownstreamConnections, which adds an overload manager to the bootstrap, so
	// the new revision is larger than the one it replaces.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gwp := &kgateway.GatewayParameters{}
		require.NoError(c, s.client.Get(ctx, key, gwp))
		gwp.Spec.Kube.EnvoyContainer.Bootstrap.MaxDownstreamConnections = new(int64(100))
		require.NoError(c, s.client.Update(ctx, gwp))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out updating GatewayParameters")

	// The reconciler records a Warning event that names the earlier revision, sets no
	// BootstrapOverlayApplied condition, and leaves the proxy Deployment on the running bootstrap.
	s.waitForGatewayEvent(key, conditions.KgatewayReasonBootstrapConfigMapTooLarge,
		"earlier bootstrap revisions that proxy pods or ReplicaSets still use")
	s.waitForBootstrapOverlayApplied(key, "", "", false)
	s.Require().NoError(s.client.Get(ctx, key, dep))
	s.Equal(runningChecksum, dep.Spec.Template.Annotations[bootstrapoverlay.ChecksumAnnotation],
		"the Deployment should keep the running bootstrap")
}

// TestGatewayClassStatus tests the Status conditions on GatewayClass
func (s *ControllerSuite) TestGatewayClassStatus() {
	ctx := context.Background()

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gc := &gwv1.GatewayClass{}
		err := s.client.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gc)
		require.NoError(c, err, "error getting GatewayClass")

		// Check Accepted condition
		condition := meta.FindStatusCondition(gc.Status.Conditions, string(gwv1.GatewayClassConditionStatusAccepted))
		require.NotNil(c, condition)
		require.Equal(c, metav1.ConditionTrue, condition.Status)
		require.Equal(c, string(gwv1.GatewayClassReasonAccepted), condition.Reason)
		require.Contains(c, condition.Message, reports.GatewayClassAcceptedMessage)
		require.Equal(c, gc.Generation, condition.ObservedGeneration)

		// Check SupportedFeatures
		require.ElementsMatch(c, gc.Status.SupportedFeatures, deployer.GetSupportedFeaturesForStandardGateway(true))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass to be present")
}

func (s *ControllerSuite) TestMetrics() {
	ctx := context.Background()
	var gw *gwv1.Gateway

	setup := func(t *testing.T) {
		r := require.New(t)
		gw = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: defaultNamespace,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: gwv1.ObjectName(gatewayClassName),
				Listeners: []gwv1.Listener{{
					Protocol: "HTTP",
					Port:     80,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: new(gwv1.NamespacesFromSame),
						},
					},
					Name: "listener",
				}},
			},
		}
		err := s.client.Create(ctx, gw)
		r.NoError(err)

		// Wait for the Gateway Service to be created
		svc := &corev1.Service{}
		r.EventuallyWithT(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, svc)
			assert.NoError(c, err, "error getting Gateway Service")
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for Gateway Service to be created")

		if !metrics.Active() {
			return
		}

		// Wait for gateway controller to reconcile and record metrics
		// Check that reconciliation metrics have been recorded for the gateway controller
		r.EventuallyWithT(func(c *assert.CollectT) {
			gathered := metricstest.MustGatherMetrics(c)
			require.Greater(c, gathered.MetricLength("kgateway_controller_reconciliations_total"), 0)
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for Gateway controller metrics to be recorded")

		probs, err := metricstest.GatherAndLint()
		r.NoError(err)
		r.Empty(probs)
	}

	s.Run("metrics generation", func() {
		t := s.T()
		t.Cleanup(func() {
			err := s.client.Delete(ctx, gw)
			assert.NoError(t, err)
		})

		// Set up the Gateway
		setup(t)

		gathered := metricstest.MustGatherMetricsContext(ctx, t,
			"kgateway_controller_reconciliations_total")

		gathered.AssertMetricsInclude("kgateway_controller_reconciliations_total", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gateway"},
					{Name: "result", Value: "success"},
				},
				Test: metricstest.GreaterOrEqual(1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gatewayclass"},
					{Name: "result", Value: "success"},
				},
				Test: metricstest.GreaterOrEqual(1),
			},
		})
		gathered.AssertMetricsLabelsInclude("kgateway_controller_reconciliations_total", [][]metrics.Label{{
			{Name: "controller", Value: "gateway"},
			{Name: "result", Value: "success"},
		}})
	})

	s.Run("metrics disabled", func() {
		t := s.T()
		metrics.SetActive(false)
		oldRegistry := metrics.Registry()
		metrics.SetRegistry(false, metrics.NewRegistry())

		t.Cleanup(func() {
			metrics.SetActive(true)
			metrics.SetRegistry(false, oldRegistry)

			err := s.client.Delete(ctx, gw)
			assert.NoError(t, err)
		})

		// Set up the Gateway
		setup(t)

		gathered := metricstest.MustGatherMetrics(t)
		gathered.AssertMetricNotExists("kgateway_controller_reconciliations_total")
	})
}

// TestGatewayClass tests the GatewayClass controller
func (s *ControllerSuite) TestGatewayClass() {
	ctx := context.Background()

	s.Run("default GatewayClasses should be created", func() {
		t := s.T()
		r := require.New(t)

		for _, gwClass := range gwClasses {
			gc := &gwv1.GatewayClass{}
			r.EventuallyWithTf(func(c *assert.CollectT) {
				err := s.client.Get(ctx, types.NamespacedName{Name: gwClass}, gc)
				assert.NoError(c, err)
			}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", gwClass)
		}
	})

	s.Run("GatewayClass owned by external controller should not be mutated", func() {
		t := s.T()
		externalController := gwv1.GatewayController("external.controller/name")
		externalGC := &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "other-controller",
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: externalController,
			},
		}
		t.Cleanup(func() {
			err := s.client.Delete(ctx, externalGC)
			assert.NoError(t, err)
		})

		r := require.New(t)
		err := s.client.Create(ctx, externalGC)
		r.NoError(err)

		// Verify our GatewayClasses are created with correct controller
		for _, gwClass := range gwClasses {
			gc := &gwv1.GatewayClass{}
			r.EventuallyWithTf(func(c *assert.CollectT) {
				err := s.client.Get(ctx, types.NamespacedName{Name: gwClass}, gc)
				assert.NoError(c, err)
				assert.Equal(c, gwv1.GatewayController(gwClassToController[gwClass]), gc.Spec.ControllerName)
			}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", gwClass)
		}
		// Verify the external GatewayClass is unaffected
		err = s.client.Get(ctx, types.NamespacedName{Name: externalGC.Name}, externalGC)
		r.NoError(err)
		r.Equal(externalController, externalGC.Spec.ControllerName)
	})

	s.Run("default GatewayClasses should be recreated on deletion", func() {
		t := s.T()
		r := require.New(t)

		for _, gwClass := range gwClasses {
			originalGC := &gwv1.GatewayClass{}
			r.EventuallyWithTf(func(c *assert.CollectT) {
				err := s.client.Get(ctx, types.NamespacedName{Name: gwClass}, originalGC)
				assert.NoError(c, err)
			}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", gwClass)

			// Delete the GatewayClass
			err := s.client.Delete(ctx, &gwv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: gwClass}})
			r.NoError(err)
			// Verify it is recreated
			r.EventuallyWithTf(func(c *assert.CollectT) {
				newGC := &gwv1.GatewayClass{}
				err := s.client.Get(ctx, types.NamespacedName{Name: gwClass}, newGC)
				assert.NoError(c, err)
				assert.NotEqual(c, newGC.UID, originalGC.UID, "expected GatewayClass to be recreated")
			}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be recreated", gwClass)
		}
	})

	s.Run("default GatewayClass managed fields should be restored and user fields kept when it is updated", func() {
		t := s.T()
		r := require.New(t)
		gwc := &gwv1.GatewayClass{}

		// Wait for default GatewayClass to be created
		r.EventuallyWithTf(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gwc)
			assert.NoError(c, err)
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", gatewayClassName)

		// Update a field the controller manages (description) and one it does not (a user annotation)
		original := gwc.DeepCopy()
		userAnnotation := "example.com/user-annotation"
		if gwc.Annotations == nil {
			gwc.Annotations = map[string]string{}
		}
		gwc.Annotations[userAnnotation] = "kept"
		gwc.Spec.Description = new("updated description")
		err := s.client.Patch(ctx, gwc, client.MergeFrom(original))
		r.NoError(err)

		// The generation bump makes the controller re-apply its desired spec via SSA, which restores
		// the description. Waiting for that proves the re-apply ran, so the user annotation must have survived it.
		r.EventuallyWithTf(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gwc)
			assert.NoError(c, err)
			assert.NotNil(c, gwc.Spec.Description)
			assert.Equal(c, "default GatewayClass", *gwc.Spec.Description)
			assert.Equal(c, "kept", gwc.Annotations[userAnnotation])
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s", gatewayClassName)
	})

	s.Run("default GatewayClass ParametersRef should be restored when changed", func() {
		t := s.T()
		r := require.New(t)
		gwc := &gwv1.GatewayClass{}

		// Wait for selfManagedGatewayClass to be created
		r.EventuallyWithTf(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: selfManagedGatewayClassName}, gwc)
			assert.NoError(c, err)
			assert.NotNil(c, gwc.Spec.ParametersRef, "expected ParametersRef to be set")
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", selfManagedGatewayClassName)

		// Store the original ParametersRef
		original := gwc.DeepCopy()

		// Change ParametersRef to something different
		gwc.Spec.ParametersRef = &gwv1.ParametersReference{
			Group:     "different.group",
			Kind:      "DifferentKind",
			Name:      "different-params",
			Namespace: new(gwv1.Namespace("different-namespace")),
		}
		err := s.client.Patch(ctx, gwc, client.MergeFrom(original))
		r.NoError(err)

		// Verify ParametersRef is restored to original value
		r.EventuallyWithTf(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: selfManagedGatewayClassName}, gwc)
			assert.NoError(c, err)
			assert.NotNil(c, gwc.Spec.ParametersRef, "expected ParametersRef to be set")
			originalParamsRef := original.Spec.ParametersRef
			assert.Equal(c, originalParamsRef.Group, gwc.Spec.ParametersRef.Group, "ParametersRef.Group should be restored")
			assert.Equal(c, originalParamsRef.Kind, gwc.Spec.ParametersRef.Kind, "ParametersRef.Kind should be restored")
			assert.Equal(c, originalParamsRef.Name, gwc.Spec.ParametersRef.Name, "ParametersRef.Name should be restored")
			if originalParamsRef.Namespace != nil {
				assert.NotNil(c, gwc.Spec.ParametersRef.Namespace, "ParametersRef.Namespace should be set")
				assert.Equal(c, *originalParamsRef.Namespace, *gwc.Spec.ParametersRef.Namespace, "ParametersRef.Namespace should be restored")
			}
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for ParametersRef to be restored for GatewayClass %s", selfManagedGatewayClassName)
	})
}

//
// Add test helpers below. All suite tests should be placed together above
//

// createGatewayWithOverlay creates a Gateway and the GatewayParameters it uses, with
// the given bootstrap overlay, both named by key, and deletes them when the test ends.
func (s *ControllerSuite) createGatewayWithOverlay(key types.NamespacedName, overlayJSON string) {
	ctx := context.Background()

	// Build the GatewayParameters and the Gateway.
	gwp := &kgateway.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				EnvoyContainer: &kgateway.EnvoyContainer{Bootstrap: &kgateway.EnvoyBootstrap{}},
			},
		},
	}
	setBootstrapOverlay(gwp, overlayJSON)
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(gatewayClassName),
			Infrastructure: &gwv1.GatewayInfrastructure{
				ParametersRef: &gwv1.LocalParametersReference{
					Group: kgateway.GroupName,
					Kind:  gwv1.Kind(wellknown.GatewayParametersGVK.Kind),
					Name:  gwp.Name,
				},
			},
			Listeners: []gwv1.Listener{{
				Name:     "listener",
				Protocol: "HTTP",
				Port:     80,
			}},
		},
	}

	// Create them, and delete them when the test ends.
	s.T().Cleanup(func() {
		s.NoError(s.client.Delete(ctx, gw))
		s.NoError(s.client.Delete(ctx, gwp))
	})
	s.Require().NoError(s.client.Create(ctx, gwp))
	s.Require().NoError(s.client.Create(ctx, gw))
}

// updateGateway applies edit to the Gateway named by key and updates it, retrying on a conflict.
func (s *ControllerSuite) updateGateway(key types.NamespacedName, edit func(gw *gwv1.Gateway)) {
	ctx := context.Background()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gw := &gwv1.Gateway{}
		require.NoError(c, s.client.Get(ctx, key, gw))
		edit(gw)
		require.NoError(c, s.client.Update(ctx, gw))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out updating Gateway")
}

// updateOverlay replaces the bootstrap overlay of the GatewayParameters named by key;
// an empty overlay removes the overlays.
func (s *ControllerSuite) updateOverlay(key types.NamespacedName, overlayJSON string) {
	ctx := context.Background()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gwp := &kgateway.GatewayParameters{}
		require.NoError(c, s.client.Get(ctx, key, gwp))
		setBootstrapOverlay(gwp, overlayJSON)
		require.NoError(c, s.client.Update(ctx, gwp))
	}, defaultPollTimeout, 500*time.Millisecond, "timed out updating GatewayParameters")
}

// waitForBootstrapOverlayApplied waits until the Gateway named by key has the
// kgateway.dev/BootstrapOverlayApplied condition with status and reason, or none when
// status is empty, and the deployer's verdict on its parameters is or is not a failure,
// which the status writer reports as Accepted=False/InvalidParameters.
func (s *ControllerSuite) waitForBootstrapOverlayApplied(key types.NamespacedName, status metav1.ConditionStatus, reason string, invalidParameters bool) {
	ctx := context.Background()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gw := &gwv1.Gateway{}
		require.NoError(c, s.client.Get(ctx, key, gw))

		// Check the bootstrap condition.
		cond := meta.FindStatusCondition(gw.Status.Conditions, conditions.KgatewayConditionBootstrapOverlayApplied)
		if status == "" {
			require.Nil(c, cond, "expected no %s condition", conditions.KgatewayConditionBootstrapOverlayApplied)
		} else {
			require.NotNil(c, cond)
			require.Equal(c, status, cond.Status)
			require.Equal(c, reason, cond.Reason)
			require.Equal(c, gw.Generation, cond.ObservedGeneration)
		}

		// Check whether the deployer rejects the Gateway's parameters.
		verdict := s.verdictFor(key)
		if invalidParameters {
			require.NotNil(c, verdict, "the deployer should publish its verdict")
			require.NotEmpty(c, verdict.Err(), "the verdict should be a failure")
			require.Equal(c, gw.Generation, verdict.ObservedGeneration())
		} else if verdict != nil {
			require.Empty(c, verdict.Err(), "the verdict should not be a failure")
		}
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for %s=%s/%s", conditions.KgatewayConditionBootstrapOverlayApplied, status, reason)
}

// waitForGatewayEvent waits until the Gateway named by key has a Warning event with
// reason whose note contains noteSubstring.
func (s *ControllerSuite) waitForGatewayEvent(key types.NamespacedName, reason, noteSubstring string) {
	ctx := context.Background()
	clientset, err := kubernetes.NewForConfig(s.env.Config)
	s.Require().NoError(err)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		// List the namespace's events.
		evts, err := clientset.EventsV1().Events(key.Namespace).List(ctx, metav1.ListOptions{})
		require.NoError(c, err)

		// Find one for the Gateway with the reason and note.
		found := slices.ContainsFunc(evts.Items, func(e eventsv1.Event) bool {
			return e.Regarding.Kind == wellknown.GatewayKind && e.Regarding.Name == key.Name &&
				e.Type == corev1.EventTypeWarning && e.Reason == reason && strings.Contains(e.Note, noteSubstring)
		})
		require.True(c, found, "no Warning event %s containing %q for Gateway %s", reason, noteSubstring, key)
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for event %s on Gateway %s", reason, key)
}

// setBootstrapOverlay sets the bootstrap overlay of gwp; an empty value removes it.
func setBootstrapOverlay(gwp *kgateway.GatewayParameters, overlayJSON string) {
	// Remove any overlay.
	gwp.Spec.Kube.EnvoyContainer.Bootstrap.Overlay = nil

	// Set the new one, if any.
	if overlayJSON != "" {
		gwp.Spec.Kube.EnvoyContainer.Bootstrap.Overlay = &kgateway.BootstrapOverlay{
			MergeStrategy: kgateway.BootstrapMergeStrategyStructuredMerge,
			Value:         apiextensionsv1.JSON{Raw: []byte(overlayJSON)},
		}
	}
}

type fakeDiscoveryNamespaceFilter struct{}

func (f fakeDiscoveryNamespaceFilter) Filter(obj any) bool {
	// this is a fake filter, so we just return true
	return true
}

func (f fakeDiscoveryNamespaceFilter) AddHandler(func(selected, deselected istiosets.String)) {}

func generateKubeconfig(restconfig *rest.Config) (string, error) {
	clusters := make(map[string]*clientcmdapi.Cluster)
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	contexts := make(map[string]*clientcmdapi.Context)

	clusterName := "cluster"
	clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   restconfig.Host,
		CertificateAuthorityData: restconfig.CAData,
	}
	authinfos[clusterName] = &clientcmdapi.AuthInfo{
		ClientKeyData:         restconfig.KeyData,
		ClientCertificateData: restconfig.CertData,
	}
	contexts[clusterName] = &clientcmdapi.Context{
		Cluster:   clusterName,
		Namespace: "default",
		AuthInfo:  clusterName,
	}

	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "cluster",
		AuthInfos:      authinfos,
	}
	// create temp file
	tmpfile, err := os.CreateTemp("", "kgw*_controller_test_kubeconfig.yaml")
	if err != nil {
		return "", fmt.Errorf("error creating tmp kubeconfig file: %w", err)
	}
	tmpfile.Close()
	err = clientcmd.WriteToFile(clientConfig, tmpfile.Name())
	if err != nil {
		return "", fmt.Errorf("error writing kubeconfig file: %w", err)
	}
	return tmpfile.Name(), nil
}

func (s *ControllerSuite) startController(
	ctx context.Context,
	cfg *rest.Config,
	scheme *runtime.Scheme,
	env *envtest.Environment,
) error {
	kubeClient, err := apiclient.New(cfg)
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    env.WebhookInstallOptions.LocalServingHost,
			Port:    env.WebhookInstallOptions.LocalServingPort,
			CertDir: env.WebhookInstallOptions.LocalServingCertDir,
		}),
		Controller: config.Controller{
			// see https://github.com/kubernetes-sigs/controller-runtime/issues/2937
			// in short, our tests reuse the same name (reasonably so) and the controller-runtime
			// package does not reset the stack of controller names between tests, so we disable
			// the name validation here.
			SkipNameValidation: new(true),
		},
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		return err
	}

	if err := s.client.Create(ctx, &kgateway.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      selfManagedGatewayClassName,
			Namespace: "default",
		},
		Spec: kgateway.GatewayParametersSpec{
			SelfManaged: &kgateway.SelfManagedGateway{},
		},
	}); client.IgnoreAlreadyExists(err) != nil {
		return err
	}

	commonCols, err := newCommonCols(ctx, kubeClient)
	if err != nil {
		return err
	}
	s.paramsStatus = reports.NewGatewayParametersStatus()

	gwCfg := GatewayConfig{
		Client:         kubeClient,
		Mgr:            mgr,
		ControllerName: gatewayControllerName,
		ImageInfo: &deployer.ImageInfo{
			Registry: "ghcr.io/kgateway-dev",
			Tag:      "latest",
		},
		DiscoveryNamespaceFilter: fakeDiscoveryNamespaceFilter{},
		CommonCollections:        commonCols,
		GatewayParametersStatus:  s.paramsStatus,
	}

	supportedFeatures := deployer.GetSupportedFeaturesForStandardGateway(true)
	classConfigs := map[string]*deployer.GatewayClassInfo{
		altGatewayClassName: {
			Description:       "alt GatewayClass",
			ControllerName:    gwClassToController[altGatewayClassName],
			SupportedFeatures: supportedFeatures,
		},
		gatewayClassName: {
			Description:       "default GatewayClass",
			ControllerName:    gwClassToController[gatewayClassName],
			SupportedFeatures: supportedFeatures,
		},
		selfManagedGatewayClassName: {
			Description:    "self-managed GatewayClass",
			ControllerName: gwClassToController[selfManagedGatewayClassName],
			ParametersRef: &gwv1.ParametersReference{
				Group:     gwv1.Group(wellknown.GatewayParametersGVK.Group),
				Kind:      gwv1.Kind(wellknown.GatewayParametersGVK.Kind),
				Name:      selfManagedGatewayClassName,
				Namespace: new(gwv1.Namespace("default")),
			},
			SupportedFeatures: supportedFeatures,
		},
	}

	if err := NewBaseGatewayController(ctx, gwCfg, classConfigs, nil, nil); err != nil {
		return err
	}
	kubeClient.RunAndWait(ctx.Done())

	s.kubeconfigPath, err = generateKubeconfig(cfg)
	if err != nil {
		return err
	}

	s.managerErr = make(chan error, 1)
	go func() {
		defer close(s.managerErr)
		mgr.GetLogger().Info("starting manager", "kubeconfig", s.kubeconfigPath)
		s.managerErr <- mgr.Start(ctx)
	}()

	// Wait for manager to be ready by checking if we can list GatewayClasses
	// This ensures the controller is fully started before tests run
	s.EventuallyWithT(func(c *assert.CollectT) {
		var gcList gwv1.GatewayClassList
		err := s.client.List(ctx, &gcList)
		assert.NoError(c, err, assert.NoError)
	}, defaultPollTimeout, 250*time.Millisecond, "timed out waiting for Manager to be ready")
	select {
	case err := <-s.managerErr:
		if err != nil {
			return fmt.Errorf("controller-manager exited before it was ready: %w", err)
		}
		return errors.New("controller-manager exited before it was ready")
	default:
	}

	return nil
}

func newCommonCols(ctx context.Context, kubeClient apiclient.Client) (*collections.CommonCollections, error) {
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	settings, err := apisettings.BuildSettings()
	if err != nil {
		return nil, fmt.Errorf("error building Settings: %w", err)
	}
	commoncol, err := collections.NewCommonCollections(ctx, krtopts, kubeClient, gatewayControllerName, *settings)
	if err != nil {
		return nil, fmt.Errorf("error building CommonCollections: %w", err)
	}

	plugins := registry.Plugins(ctx, commoncol, *settings, nil)
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
	extensions := registry.MergePlugins(plugins...)

	commoncol.InitPlugins(ctx, extensions, *settings)
	return commoncol, nil
}

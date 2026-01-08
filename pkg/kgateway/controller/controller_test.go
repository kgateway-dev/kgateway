package controller

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	istiosets "istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

const (
	gatewayClassName            = "clsname"
	altGatewayClassName         = "clsname-alt"
	selfManagedGatewayClassName = "clsname-selfmanaged"
	gatewayControllerName       = "kgateway.dev/kgateway"
	agwControllerName           = "agentgateway.dev/agentgateway"
	defaultNamespace            = "default"

	localhost = "127.0.0.1"
)

// gwClasses maps the default GatewayClasses initialized in startController
var (
	gwClasses           = []string{gatewayClassName, altGatewayClassName, selfManagedGatewayClassName}
	gwClassToController = map[string]string{
		gatewayClassName:            gatewayControllerName,
		altGatewayClassName:         agwControllerName,
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
}

func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerSuite))
}

func (s *ControllerSuite) SetupSuite() {
	// Don't use the testing.T.Context because it is cancelled before the corresponding
	// Cleanup function is called, and we need the Client/Manager to be alive in t.Cleanup handlers
	ctx, cancel := context.WithCancel(context.Background())
	s.suitCtxCancelFn = cancel

	// Create a scheme and add both Gateway and InferencePool types.
	scheme := schemes.GatewayScheme()
	err := inf.Install(scheme)
	s.Require().NoError(err)

	// Required to deploy endpoint picker RBAC resources.
	err = rbacv1.AddToScheme(scheme)
	s.Require().NoError(err)

	assetsDir, err := getAssetsDir()
	s.Require().NoError(err)

	s.env = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "crds"),
			filepath.Join("..", "..", "..", "install", "helm", "kgateway-crds", "templates"),
			filepath.Join("..", "..", "..", "install", "helm", "agentgateway-crds", "templates"),
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
		s.T().Run(tc.name, func(t *testing.T) {
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
						Type:  ptr.To(gwv1.IPAddressType),
						Value: localhost,
					}},
					GatewayClassName: gwv1.ObjectName(tc.gatewayClass),
					Listeners: []gwv1.Listener{{
						Protocol: "HTTP",
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: ptr.To(gwv1.NamespacesFromSame),
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
					Replicas: ptr.To[int32](2),
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

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		err := s.client.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, gw)
		require.NoError(c, err, "error getting Gateway")

		condition := meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionAccepted))
		require.NotNil(c, condition)
		require.Equal(c, metav1.ConditionFalse, condition.Status)
		require.Equal(c, string(gwv1.GatewayReasonInvalidParameters), condition.Reason)
		require.Equal(c, gw.Generation, condition.ObservedGeneration)
	}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for Gateway to have GatewayReasonInvalidParameters")
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
		require.ElementsMatch(c, gc.Status.SupportedFeatures, deployer.GetSupportedFeaturesForStandardGateway())
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
							From: ptr.To(gwv1.NamespacesFromSame),
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

	s.T().Run("metrics generation", func(t *testing.T) {
		t.Cleanup(func() {
			err := s.client.Delete(ctx, gw)
			s.NoError(err)
		})

		// Set up the Gateway
		setup(t)

		gathered := metricstest.MustGatherMetricsContext(ctx, t,
			"kgateway_controller_reconciliations_total",
			"kgateway_controller_reconciliations_running",
			"kgateway_controller_reconcile_duration_seconds")

		gathered.AssertMetricsInclude("kgateway_controller_reconciliations_total", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gateway"},
					{Name: "namespace", Value: defaultNamespace},
					{Name: "name", Value: gw.Name},
					{Name: "result", Value: "success"},
				},
				Test: metricstest.GreaterOrEqual(1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gatewayclass"},
					{Name: "namespace", Value: defaultNamespace},
					{Name: "name", Value: gw.Name},
					{Name: "result", Value: "success"},
				},
				Test: metricstest.GreaterOrEqual(1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gatewayclass-provisioner"},
					{Name: "namespace", Value: defaultNamespace},
					{Name: "name", Value: gw.Name},
					{Name: "result", Value: "success"},
				},
				Test: metricstest.GreaterOrEqual(1),
			},
		})

		gathered.AssertMetricsInclude("kgateway_controller_reconciliations_running", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gateway"},
					{Name: "name", Value: gw.Name},
					{Name: "namespace", Value: defaultNamespace},
				},
				Test: metricstest.Between(0, 1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gatewayclass"},
					{Name: "name", Value: gw.Name},
					{Name: "namespace", Value: defaultNamespace},
				},
				Test: metricstest.Between(0, 1),
			},
			&metricstest.ExpectedMetricValueTest{
				Labels: []metrics.Label{
					{Name: "controller", Value: "gatewayclass-provisioner"},
					{Name: "name", Value: gw.Name},
					{Name: "namespace", Value: defaultNamespace},
				},
				Test: metricstest.Between(0, 1),
			},
		})

		gathered.AssertMetricsLabelsInclude("kgateway_controller_reconcile_duration_seconds", [][]metrics.Label{{
			{Name: "controller", Value: "gateway"},
			{Name: "name", Value: gw.Name},
			{Name: "namespace", Value: defaultNamespace},
		}})
	})

	s.T().Run("metrics disabled", func(t *testing.T) {
		metrics.SetActive(false)
		oldRegistry := metrics.Registry()
		metrics.SetRegistry(false, metrics.NewRegistry())

		t.Cleanup(func() {
			metrics.SetActive(true)
			metrics.SetRegistry(false, oldRegistry)

			err := s.client.Delete(ctx, gw)
			s.NoError(err)
		})

		// Set up the Gateway
		setup(t)

		gathered := metricstest.MustGatherMetrics(t)
		gathered.AssertMetricNotExists("kgateway_controller_reconciliations_total")
		gathered.AssertMetricNotExists("kgateway_controller_reconciliations_running")
		gathered.AssertMetricNotExists("kgateway_controller_reconcile_duration_seconds")
	})
}

// TestGatewayClass tests the GatewayClass controller
func (s *ControllerSuite) TestGatewayClass() {
	ctx := context.Background()

	s.T().Run("default GatewayClasses should be created", func(t *testing.T) {
		r := require.New(t)

		for _, gwClass := range gwClasses {
			gc := &gwv1.GatewayClass{}
			r.EventuallyWithTf(func(c *assert.CollectT) {
				err := s.client.Get(ctx, types.NamespacedName{Name: gwClass}, gc)
				assert.NoError(c, err)
			}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", gwClass)
		}
	})

	s.T().Run("GatewayClass owned by external controller should not be mutated", func(t *testing.T) {
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
			s.NoError(err)
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

	s.T().Run("default GatewayClasses should be recreated on deletion", func(t *testing.T) {
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

	s.T().Run("default GatewayClass should not be overwritten when it is updated", func(t *testing.T) {
		r := require.New(t)
		gwc := &gwv1.GatewayClass{}

		// Wait for default GatewayClass to be created
		r.EventuallyWithTf(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gwc)
			assert.NoError(c, err)
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s to be created", gatewayClassName)

		// Update it
		original := gwc.DeepCopy()
		updatedDesc := "updated description"
		gwc.Spec.Description = ptr.To(updatedDesc)
		err := s.client.Patch(ctx, gwc, client.MergeFrom(original))
		r.NoError(err)

		// Verify it is not overwritten
		r.EventuallyWithTf(func(c *assert.CollectT) {
			err := s.client.Get(ctx, types.NamespacedName{Name: gatewayClassName}, gwc)
			assert.NoError(c, err)
			assert.NotNil(c, gwc.Spec.Description)
			assert.Equal(c, updatedDesc, *gwc.Spec.Description)
		}, defaultPollTimeout, 500*time.Millisecond, "timed out waiting for GatewayClass %s", gatewayClassName)
	})

	s.T().Run("default GatewayClass ParametersRef should be restored when changed", func(t *testing.T) {
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
			Namespace: ptr.To(gwv1.Namespace("different-namespace")),
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

type fakeDiscoveryNamespaceFilter struct{}

func (f fakeDiscoveryNamespaceFilter) Filter(obj any) bool {
	// this is a fake filter, so we just return true
	return true
}

func (f fakeDiscoveryNamespaceFilter) AddHandler(func(selected, deselected istiosets.String)) {}

func getAssetsDir() (string, error) {
	var assets string
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// set default if not user provided
		out, err := exec.Command("sh", "-c", "make -s --no-print-directory -C $(dirname $(go env GOMOD)) envtest-path").CombinedOutput()
		if err != nil {
			return "", err
		}
		assets = strings.TrimSpace(string(out))
	}
	if assets != "" {
		info, err := os.Stat(assets)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("assets path is not a directory: %s", assets)
		}
	}
	return assets, nil
}

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
			SkipNameValidation: ptr.To(true),
		},
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		return err
	}

	if err := mgr.GetClient().Create(ctx, &kgateway.GatewayParameters{
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

	gwCfg := GatewayConfig{
		Client:             kubeClient,
		Mgr:                mgr,
		ControllerName:     gatewayControllerName,
		AgwControllerName:  agwControllerName,
		EnableEnvoy:        true,
		EnableAgentgateway: true,
		ImageInfo: &deployer.ImageInfo{
			Registry: "ghcr.io/kgateway-dev",
			Tag:      "latest",
		},
		DiscoveryNamespaceFilter: fakeDiscoveryNamespaceFilter{},
		CommonCollections:        commonCols,
	}

	supportedFeatures := deployer.GetSupportedFeaturesForStandardGateway()
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
				Namespace: ptr.To(gwv1.Namespace("default")),
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

	go func() {
		mgr.GetLogger().Info("starting manager", "kubeconfig", s.kubeconfigPath)
		err := mgr.Start(ctx)
		s.Require().NoError(err, "error starting controller-manager")
	}()

	// Wait for manager to be ready by checking if we can list GatewayClasses
	// This ensures the controller is fully started before tests run
	s.EventuallyWithT(func(c *assert.CollectT) {
		var gcList gwv1.GatewayClassList
		err := mgr.GetClient().List(ctx, &gcList)
		assert.NoError(c, err, assert.NoError)
	}, defaultPollTimeout, 250*time.Millisecond, "timed out waiting for Manager to be ready")

	return nil
}

func newCommonCols(ctx context.Context, kubeClient apiclient.Client) (*collections.CommonCollections, error) {
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	settings, err := apisettings.BuildSettings()
	if err != nil {
		return nil, fmt.Errorf("error building Settings: %w", err)
	}
	commoncol, err := collections.NewCommonCollections(ctx, krtopts, kubeClient, gatewayControllerName, agwControllerName, *settings)
	if err != nil {
		return nil, fmt.Errorf("error building CommonCollections: %w", err)
	}

	plugins := registry.Plugins(ctx, commoncol, wellknown.DefaultWaypointClassName, *settings, nil)
	plugins = append(plugins, krtcollections.NewBuiltinPlugin(ctx))
	extensions := registry.MergePlugins(plugins...)

	commoncol.InitPlugins(ctx, extensions, *settings)
	return commoncol, nil
}

//go:build e2e

package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{defaults.HttpbinManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestProvisionDeploymentAndService": {
			Manifests: []string{gatewayWithoutParameters},
		},
		"TestConfigureProxiesFromGatewayParameters": {
			Manifests: []string{gatewayParametersCustom, gatewayWithParameters},
		},
		"TestProvisionResourcesUpdatedWithValidParameters": {
			Manifests: []string{gatewayWithParameters},
		},
		"TestMissingGatewayParameters": {
			Manifests: []string{gatewayWithoutParameters},
		},
		"TestProvisionResourcesNotUpdatedWithInvalidParameters": {
			Manifests: []string{gatewayWithParameters},
		},
		"TestSelfManagedGateway": {
			Manifests: []string{selfManagedGateway},
		},
		"TestGatewayParametersUpdateTriggersReconciliation": {
			Manifests: []string{gatewayWithParameters},
		},
		"TestHPAPDBLifecycle": {
			Manifests: []string{gatewayWithHPAPDB},
		},
	}
)

// testingSuite is the entire Suite of tests for the "deployer" feature
// The "deployer" code can be found here: /pkg/kgateway/deployer
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	// Register PDB and HPA types in the client's scheme so that tests
	// can use the typed client to get/list these resources.
	scheme := s.TestInstallation.ClusterContext.Client.Scheme()
	_ = policyv1.AddToScheme(scheme)
	_ = autoscalingv2.AddToScheme(scheme)

	// Install VPA CRD so the deployer can create VPA resources.
	err := s.TestInstallation.ClusterContext.IstioClient.ApplyYAMLFiles("", vpaCRDManifest)
	s.Require().NoError(err, "failed to install VPA CRD")

	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TearDownSuite() {
	s.BaseTestingSuite.TearDownSuite()

	// Clean up the VPA CRD installed in SetupSuite
	if !testutils.ShouldSkipCleanup(s.T()) {
		_ = s.TestInstallation.ClusterContext.IstioClient.DeleteYAMLFiles("", vpaCRDManifest)
	}
}

func (s *testingSuite) TestProvisionDeploymentAndService() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))
}

func (s *testingSuite) TestConfigureProxiesFromGatewayParameters() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// check that the labels and annotations got passed through from GatewayParameters to the ServiceAccount
	sa := &corev1.ServiceAccount{}
	err := s.TestInstallation.ClusterContext.Client.Get(
		s.Ctx,
		client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		},
		sa,
	)
	s.Require().NoError(err)

	s.TestInstallation.AssertionsT(s.T()).Gomega.Expect(sa.GetLabels()).To(
		gomega.HaveKeyWithValue("sa-label-key", "sa-label-val"))
	s.TestInstallation.AssertionsT(s.T()).Gomega.Expect(sa.GetAnnotations()).To(
		gomega.HaveKeyWithValue("sa-anno-key", "sa-anno-val"))

	// check that the labels and annotations got passed through from GatewayParameters to the Service
	svc := &corev1.Service{}
	err = s.TestInstallation.ClusterContext.Client.Get(
		s.Ctx,
		client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		},
		svc,
	)
	s.Require().NoError(err)
	s.TestInstallation.AssertionsT(s.T()).Gomega.Expect(svc.GetLabels()).To(
		gomega.HaveKeyWithValue("svc-label-key", "svc-label-val"))
	s.TestInstallation.AssertionsT(s.T()).Gomega.Expect(svc.GetAnnotations()).To(
		gomega.HaveKeyWithValue("svc-anno-key", "svc-anno-val"))

	// check that the proxy pod has the expected labels
	pods, err := kubeutils.GetReadyPodsForDeployment(s.Ctx, s.TestInstallation.ClusterContext.Clientset, proxyObjectMeta)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)
	pod := &corev1.Pod{}
	err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      pods[0],
	}, pod)
	s.Require().NoError(err)
	s.Require().Subset(pod.Labels, map[string]string{
		"app.kubernetes.io/instance":             proxyObjectMeta.Name,
		defaults.WellKnownAppLabel:               proxyObjectMeta.Name,
		"gateway.networking.k8s.io/gateway-name": proxyObjectMeta.Name,
		"kgateway":                               "kube-gateway",
	})

	// check that the rollout strategy is as expected
	proxyDeployment := &appsv1.Deployment{}
	err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, proxyDeployment)
	s.Require().NoError(err)
	s.TestInstallation.AssertionsT(s.T()).Gomega.Expect(proxyDeployment.Spec.Strategy).To(gomega.Equal(
		appsv1.DeploymentStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "1%"},
				MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
			},
		}))

	// Update the Gateway to use the custom GatewayParameters
	gw := &gwv1.Gateway{}
	err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, gw)
	s.Require().NoError(err)
	s.patchGateway(proxyObjectMeta, func(gw *gwv1.Gateway) {
		gw.Spec.Infrastructure.ParametersRef = &gwv1.LocalParametersReference{
			Group: "gateway.kgateway.dev",
			Kind:  "GatewayParameters",
			Name:  gwParamsCustomObjectMeta.Name,
		}
	})

	// Assert that the expected custom configuration exists.
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(2))

	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		serverInfoLogLevelAssertion(s.T(), s.TestInstallation, "debug", "connection:trace,upstream:debug"),
		xdsClusterAssertion(s.T(), s.TestInstallation),
	)
}

func (s *testingSuite) TestProvisionResourcesUpdatedWithValidParameters() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// modify the number of replicas in the GatewayParameters
	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *kgateway.GatewayParameters) {
		parameters.Spec.Kube.Deployment.Replicas = ptr.To[int32](2)
	})

	// the GatewayParameters modification should cause the deployer to re-run and update the
	// deployment to have 2 replicas
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(2))
}

// TestMissingGatewayParameters tests that a Gateway referencing a missing GatewayParameters
// is not accepted, and once the GatewayParameters is created, the Gateway is accepted.
// This is to make sure that the controller and status syncer are working properly
// until this is fixed: https://github.com/kgateway-dev/kgateway/issues/12207
func (s *testingSuite) TestMissingGatewayParameters() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// patch the Gateway to reference a missing GatewayParameters
	gw := &gwv1.Gateway{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, gw)
	s.Require().NoError(err)
	s.patchGateway(proxyObjectMeta, func(gw *gwv1.Gateway) {
		gw.Spec.Infrastructure = &gwv1.GatewayInfrastructure{}
		gw.Spec.Infrastructure.ParametersRef = &gwv1.LocalParametersReference{
			Group: "gateway.kgateway.dev",
			Kind:  "GatewayParameters",
			Name:  gwParamsCustomObjectMeta.Name,
		}
	})

	// ensure the gateway pod is still running
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// initially, the Gateway should not be accepted
	// because the GatewayParameters is missing
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		proxyObjectMeta.Name,
		proxyObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionFalse,
	)

	testutils.Cleanup(s.T(), func() {
		s.DeleteManifests(&base.TestCase{
			Manifests: []string{gatewayParametersCustom},
		})
	})

	// create the missing GatewayParameters
	s.ApplyManifests(&base.TestCase{
		Manifests: []string{gatewayParametersCustom},
	})

	// the Gateway should be accepted once the GatewayParameters is created
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		proxyObjectMeta.Name,
		proxyObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
}

func (s *testingSuite) TestProvisionResourcesNotUpdatedWithInvalidParameters() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	proxyDeployment := &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, proxyDeployment)
	s.Require().NoError(err)

	// initially, allowPrivilegeEscalation should be true and privileged should not be set
	origAllowPrivilegeEscalation := proxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation
	s.Assert().NotNil(origAllowPrivilegeEscalation)
	s.Assert().True(*origAllowPrivilegeEscalation)
	origPrivileged := proxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.Privileged
	s.Assert().Nil(origPrivileged)

	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *kgateway.GatewayParameters) {
		// try to modify GatewayParameters with invalid values
		// K8s won't allow setting both allowPrivilegeEscalation=false and privileged=true,
		// so the proposed patch should fail and the original values should be retained.
		parameters.Spec.Kube.EnvoyContainer = &kgateway.EnvoyContainer{
			SecurityContext: &corev1.SecurityContext{
				Privileged:               new(true),
				AllowPrivilegeEscalation: new(false),
			},
		}

		// This is valid, but should be ignored, because another part of this patch is invalid
		parameters.Spec.Kube.Deployment.Replicas = ptr.To[int32](2)
	})

	// We keep checking for some amount of time (30s) to account for the time it might take for
	// the deployer to run and re-provision resources. If the original values are consistently
	// retained after that amount of time, we can be confident that the deployer has had time to
	// consume the new values and fail to apply them.
	s.TestInstallation.AssertionsT(s.T()).Gomega.Consistently(func(g gomega.Gomega) {
		proxyDeployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, proxyDeployment)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(proxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation).To(gomega.Equal(origAllowPrivilegeEscalation))
		g.Expect(proxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.Privileged).To(gomega.Equal(origPrivileged))
		g.Expect(proxyDeployment.Spec.Replicas).To(gstruct.PointTo(gomega.Equal(int32(1))))
	}, "30s", "1s").Should(gomega.Succeed())
}

func (s *testingSuite) TestSelfManagedGateway() {
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx,
			types.NamespacedName{Name: proxyObjectMeta.Name, Namespace: proxyObjectMeta.Namespace},
			gw)
		assert.NoError(c, err, "gateway not found")

		accepted := false
		for _, conditions := range gw.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayConditionAccepted) && conditions.Status == metav1.ConditionTrue {
				accepted = true
				break
			}
		}
		if !accepted {
			// Provide more context about the current gateway conditions for debugging
			fmt.Printf("Gateway not accepted. Current conditions: %v\n", gw.Status.Conditions)
		}
		assert.True(c, accepted, "gateway status not accepted")
	}, 60*time.Second, 1*time.Second)

	s.TestInstallation.AssertionsT(s.T()).ConsistentlyObjectsNotExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: proxyObjectMeta},
		&corev1.Service{ObjectMeta: proxyObjectMeta},
		&corev1.ServiceAccount{ObjectMeta: proxyObjectMeta},
	)
}

func (s *testingSuite) TestGatewayParametersUpdateTriggersReconciliation() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// Patch GatewayParameters to add a new pod annotation
	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *kgateway.GatewayParameters) {
		if parameters.Spec.Kube.PodTemplate == nil {
			parameters.Spec.Kube.PodTemplate = &kgateway.Pod{}
		}
		if parameters.Spec.Kube.PodTemplate.ExtraAnnotations == nil {
			parameters.Spec.Kube.PodTemplate.ExtraAnnotations = map[string]string{}
		}
		parameters.Spec.Kube.PodTemplate.ExtraAnnotations["test-reconcile-annotation"] = "updated"
	})

	// Verify that the deployment's pod template annotations are updated,
	// which proves that the GatewayParameters change triggered reconciliation
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		proxyDeployment := &appsv1.Deployment{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, proxyDeployment)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(proxyDeployment.Spec.Template.Annotations).To(
			gomega.HaveKeyWithValue("test-reconcile-annotation", "updated"),
			"pod template should have the annotation added via GatewayParameters patch",
		)
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())
}

// TestHPAPDBLifecycle tests the full lifecycle of HPA, PDB, and VPA resources:
// 1. Creates a Gateway with HPA, PDB, and VPA configured
// 2. Verifies the proxy deployment is healthy and all three resources exist
// 3. Updates the PDB configuration and verifies the change propagates
// 4. Removes the HPA and VPA from GatewayParameters and verifies they are pruned
// 5. Deletes the Gateway and verifies all resources are cleaned up
func (s *testingSuite) TestHPAPDBLifecycle() {
	vpaClient := s.TestInstallation.ClusterContext.IstioClient.Dynamic().
		Resource(wellknown.VerticalPodAutoscalerGVR).
		Namespace(proxyObjectMeta.Namespace)

	// Step 1: Wait for the proxy deployment to be healthy
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// Step 2: Verify PDB, HPA, and VPA were created
	pdb := &policyv1.PodDisruptionBudget{}
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, pdb)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "PDB should exist")
		g.Expect(pdb.Spec.MinAvailable).NotTo(gomega.BeNil(), "PDB should have minAvailable set")
		g.Expect(pdb.Spec.MinAvailable.IntValue()).To(gomega.Equal(1), "PDB minAvailable should be 1")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, hpa)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "HPA should exist")
		g.Expect(hpa.Spec.MaxReplicas).To(gomega.Equal(int32(3)), "HPA maxReplicas should be 3")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		vpa, err := vpaClient.Get(s.Ctx, proxyObjectMeta.Name, metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred(), "VPA should exist")
		updateMode, _, _ := unstructured.NestedString(vpa.Object, "spec", "updatePolicy", "updateMode")
		g.Expect(updateMode).To(gomega.Equal("Off"), "VPA updateMode should be Off")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	// Step 3: Update PDB configuration - change minAvailable from 1 to 0
	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *kgateway.GatewayParameters) {
		parameters.Spec.Kube.PodDisruptionBudget = &shared.KubernetesResourceOverlay{
			Spec: mustMarshalJSON(map[string]any{
				"minAvailable": 0,
			}),
		}
	})

	// Verify PDB was updated
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		updatedPDB := &policyv1.PodDisruptionBudget{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, updatedPDB)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "PDB should still exist after update")
		g.Expect(updatedPDB.Spec.MinAvailable).NotTo(gomega.BeNil())
		g.Expect(updatedPDB.Spec.MinAvailable.IntValue()).To(gomega.Equal(0), "PDB minAvailable should be updated to 0")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	// Step 4: Remove HPA and VPA from GatewayParameters (keep PDB)
	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *kgateway.GatewayParameters) {
		parameters.Spec.Kube.HorizontalPodAutoscaler = nil
		parameters.Spec.Kube.VerticalPodAutoscaler = nil
	})

	// Verify HPA is pruned
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, &autoscalingv2.HorizontalPodAutoscaler{})
		g.Expect(err).To(gomega.HaveOccurred(), "HPA should be deleted")
		g.Expect(client.IgnoreNotFound(err)).To(gomega.Succeed(), "error should be NotFound")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	// Verify VPA is pruned
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		_, err := vpaClient.Get(s.Ctx, proxyObjectMeta.Name, metav1.GetOptions{})
		g.Expect(err).To(gomega.HaveOccurred(), "VPA should be deleted")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	// PDB should still exist
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, &policyv1.PodDisruptionBudget{})
	s.Require().NoError(err, "PDB should still exist after HPA/VPA removal")

	// Deployment should still be healthy
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// Step 5: Delete the Gateway and verify all resources are cleaned up
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		gw := &gwv1.Gateway{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
			Namespace: proxyObjectMeta.Namespace,
			Name:      proxyObjectMeta.Name,
		}, gw)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		err = s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, gw)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())

	// Wait for all owned resources to be garbage collected
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: proxyObjectMeta},
		&corev1.Service{ObjectMeta: proxyObjectMeta},
		&corev1.ServiceAccount{ObjectMeta: proxyObjectMeta},
		&policyv1.PodDisruptionBudget{ObjectMeta: proxyObjectMeta},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: proxyObjectMeta},
	)

	// VPA cleanup check (dynamic client since VPA is a CRD)
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		_, err := vpaClient.Get(s.Ctx, proxyObjectMeta.Name, metav1.GetOptions{})
		g.Expect(err).To(gomega.HaveOccurred(), "VPA should not exist after Gateway deletion")
	}).WithTimeout(60 * time.Second).WithPolling(1 * time.Second).Should(gomega.Succeed())
}

// patchGateway accepts a reference to an object, and a patch function. It then queries the object,
// performs the patch in memory, and writes the object back to the cluster.
func (s *testingSuite) patchGateway(objectMeta metav1.ObjectMeta, patchFn func(*gwv1.Gateway)) {
	gw := new(gwv1.Gateway)
	gwName := types.NamespacedName{
		Namespace: objectMeta.GetNamespace(),
		Name:      objectMeta.GetName(),
	}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, gwName, gw)
	s.Assert().NoError(err, "can get the Gateway object")
	updated := gw.DeepCopy()

	patchFn(updated)

	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, updated, client.MergeFrom(gw))
	s.Assert().NoError(err, "can update the Gateway object")
}

// patchGatewayParameters accepts a reference to an object, and a patch function
// It then queries the object, performs the patch in memory, and writes the object back to the cluster
func (s *testingSuite) patchGatewayParameters(objectMeta metav1.ObjectMeta, patchFn func(*kgateway.GatewayParameters)) {
	gatewayParameters := &kgateway.GatewayParameters{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Name:      objectMeta.GetName(),
		Namespace: objectMeta.GetNamespace(),
	}, gatewayParameters)
	s.Assert().NoError(err, "can query the GatewayParameters object")
	modifiedGatewayParameters := gatewayParameters.DeepCopy()

	patchFn(modifiedGatewayParameters)

	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, modifiedGatewayParameters, client.MergeFrom(gatewayParameters))
	s.Assert().NoError(err, "can update the GatewayParameters object")
}

func serverInfoLogLevelAssertion(t *testing.T, testInstallation *e2e.TestInstallation, expectedLogLevel, expectedComponentLogLevel string) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.AssertionsT(t).Gomega.Eventually(func(g gomega.Gomega) {
			serverInfo, err := adminClient.GetServerInfo(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get server info")
			g.Expect(serverInfo.GetCommandLineOptions().GetLogLevel()).To(
				gomega.Equal(expectedLogLevel), "defined on the GatewayParameters CR")
			g.Expect(serverInfo.GetCommandLineOptions().GetComponentLogLevel()).To(
				gomega.Equal(expectedComponentLogLevel), "defined on the GatewayParameters CR")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

func mustMarshalJSON(v any) *apiextensionsv1.JSON {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return &apiextensionsv1.JSON{Raw: data}
}

func xdsClusterAssertion(t *testing.T, testInstallation *e2e.TestInstallation) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.AssertionsT(t).Gomega.Eventually(func(g gomega.Gomega) {
			clusters, err := adminClient.GetStaticClusters(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get static clusters from config dump")

			xdsCluster, ok := clusters["xds_cluster"]
			g.Expect(ok).To(gomega.BeTrue(), "xds_cluster in list")

			g.Expect(xdsCluster.GetLoadAssignment().GetEndpoints()).To(gomega.HaveLen(1))
			g.Expect(xdsCluster.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()).To(gomega.HaveLen(1))
			xdsSocketAddress := xdsCluster.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress()
			g.Expect(xdsSocketAddress).NotTo(gomega.BeNil())

			g.Expect(xdsSocketAddress.GetAddress()).To(gomega.Equal(
				fmt.Sprintf("%s.%s.svc.cluster.local", wellknown.DefaultXdsService, testInstallation.Metadata.InstallNamespace),
			), "xds socket address points to kgateway service, in installation namespace")

			g.Expect(xdsSocketAddress.GetPortValue()).To(gomega.Equal(wellknown.DefaultXdsPort),
				"xds socket port points to kgateway service, in installation namespace")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

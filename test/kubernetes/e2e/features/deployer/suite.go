package deployer

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
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
		"TestProvisionResourcesNotUpdatedWithInvalidParameters": {
			Manifests: []string{gatewayWithParameters},
		},
		"TestSelfManagedGateway": {
			Manifests: []string{selfManagedGateway},
		},
	}
)

// testingSuite is the entire Suite of tests for the "deployer" feature
// The "deployer" code can be found here: /internal/kgateway/deployer
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestProvisionDeploymentAndService() {
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))
}

func (s *testingSuite) TestConfigureProxiesFromGatewayParameters() {
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

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

	s.TestInstallation.Assertions.Gomega.Expect(sa.GetLabels()).To(
		gomega.HaveKeyWithValue("sa-label-key", "sa-label-val"))
	s.TestInstallation.Assertions.Gomega.Expect(sa.GetAnnotations()).To(
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
	s.TestInstallation.Assertions.Gomega.Expect(svc.GetLabels()).To(
		gomega.HaveKeyWithValue("svc-label-key", "svc-label-val"))
	s.TestInstallation.Assertions.Gomega.Expect(svc.GetAnnotations()).To(
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
		"app.kubernetes.io/name":                 proxyObjectMeta.Name,
		"gateway.networking.k8s.io/gateway-name": proxyObjectMeta.Name,
		"kgateway":                               "kube-gateway",
	})

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
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(2))

	s.TestInstallation.Assertions.AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		serverInfoLogLevelAssertion(s.TestInstallation, "debug", "connection:trace,upstream:debug"),
		xdsClusterAssertion(s.TestInstallation),
	)
}

func (s *testingSuite) TestProvisionResourcesUpdatedWithValidParameters() {
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	// modify the number of replicas in the GatewayParameters
	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *v1alpha1.GatewayParameters) {
		parameters.Spec.Kube.Deployment.Replicas = ptr.To(uint32(2))
	})

	// the GatewayParameters modification should cause the deployer to re-run and update the
	// deployment to have 2 replicas
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(2))
}

func (s *testingSuite) TestProvisionResourcesNotUpdatedWithInvalidParameters() {
	s.TestInstallation.Assertions.EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

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

	s.patchGatewayParameters(gwParamsDefaultObjectMeta, func(parameters *v1alpha1.GatewayParameters) {
		// try to modify GatewayParameters with invalid values
		// K8s won't allow setting both allowPrivilegeEscalation=false and privileged=true,
		// so the proposed patch should fail and the original values should be retained.
		parameters.Spec.Kube.EnvoyContainer = &v1alpha1.EnvoyContainer{
			SecurityContext: &corev1.SecurityContext{
				Privileged:               ptr.To(true),
				AllowPrivilegeEscalation: ptr.To(false),
			},
		}

		// This is valid, but should be ignored, because another part of this patch is invalid
		parameters.Spec.Kube.Deployment.Replicas = ptr.To(uint32(2))
	})

	// We keep checking for some amount of time (30s) to account for the time it might take for
	// the deployer to run and re-provision resources. If the original values are consistently
	// retained after that amount of time, we can be confident that the deployer has had time to
	// consume the new values and fail to apply them.
	s.TestInstallation.Assertions.Gomega.Consistently(func(g gomega.Gomega) {
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

	s.TestInstallation.Assertions.ConsistentlyObjectsNotExist(s.Ctx,
		&appsv1.Deployment{ObjectMeta: proxyObjectMeta},
		&corev1.Service{ObjectMeta: proxyObjectMeta},
		&corev1.ServiceAccount{ObjectMeta: proxyObjectMeta},
	)
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
func (s *testingSuite) patchGatewayParameters(objectMeta metav1.ObjectMeta, patchFn func(*v1alpha1.GatewayParameters)) {
	gatewayParameters := &v1alpha1.GatewayParameters{}
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

func serverInfoLogLevelAssertion(testInstallation *e2e.TestInstallation, expectedLogLevel, expectedComponentLogLevel string) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
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

func xdsClusterAssertion(testInstallation *e2e.TestInstallation) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
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

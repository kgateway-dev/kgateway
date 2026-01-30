//go:build e2e

package backendtls

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var (
	configMapManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata/configmap.yaml")
	backendTLSPolicyMissingTargetManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata/missing-target.yaml")

	proxyObjMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	proxyService     = &corev1.Service{ObjectMeta: proxyObjMeta}
	backendTlsPolicy = &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-policy",
			Namespace: "default",
		},
	}
	nginxMeta = metav1.ObjectMeta{
		Name:      "nginx",
		Namespace: "default",
	}
	nginx2Meta = metav1.ObjectMeta{
		Name:      "nginx2",
		Namespace: "default",
	}
	svcGroup = ""
	svcKind  = "Service"

	// base setup manifests (shared between regular and agentgateway)
	baseSetupManifests = []string{
		filepath.Join(fsutils.MustGetThisDir(), "testdata/nginx.yaml"),
		defaults.CurlPodManifest,
		configMapManifest,
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestBackendTLSPolicyAndStatus": {},
	}
)

type tsuite struct {
	*base.BaseTestingSuite
	agentgateway bool
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: append([]string{filepath.Join(fsutils.MustGetThisDir(), "testdata/base.yaml")}, baseSetupManifests...),
	}
	return &tsuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases, base.WithMinGwApiVersion(base.GwApiRequireBackendTLSPolicy)),
		agentgateway:     false,
	}
}

func NewAgentgatewayTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: append([]string{filepath.Join(fsutils.MustGetThisDir(), "testdata/base-agw.yaml")}, baseSetupManifests...),
	}

	return &tsuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases, base.WithMinGwApiVersion(base.GwApiRequireBackendTLSPolicy)),
		agentgateway:     true,
	}
}

func (s *tsuite) TestBackendTLSPolicyAndStatus() {
	// Load the BackendTLSPolicy before proceeding with tests
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKeyFromObject(backendTlsPolicy), backendTlsPolicy)
	s.Require().NoError(err)

	tt := []struct {
		host string
	}{
		{
			host: "example.com",
		},
		{
			host: "example2.com",
		},
	}
	for _, tc := range tt {
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
				curl.WithHostHeader(tc.host),
				curl.WithPath("/"),
			},
			&matchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gomega.ContainSubstring(defaults.NginxResponse),
			},
		)
	}

	expectedStatus := http.StatusNotFound
	if s.agentgateway {
		// agentgateway does auto host rewrite
		expectedStatus = http.StatusMovedPermanently
	}
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("foo.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{
			// google return 404 this when going to google.com  with host header of "foo.com"
			StatusCode: expectedStatus,
		},
	)

	if s.agentgateway {
		// Agentgateway currently doesn't support Statuses for BackendTLSPolicy
		s.T().Log("Skipping status assertions for Agentgateway as they are not currently supported")
		return
	}
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(shared.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(shared.PolicyReasonValid),
		Message:            reports.PolicyAcceptedMsg,
		ObservedGeneration: backendTlsPolicy.Generation,
	})
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(shared.PolicyConditionAttached),
		Status:             metav1.ConditionTrue,
		Reason:             string(shared.PolicyReasonAttached),
		Message:            reports.PolicyAttachedMsg,
		ObservedGeneration: backendTlsPolicy.Generation,
	})

	// delete configmap so we can assert status updates correctly
	err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, configMapManifest)
	s.Require().NoError(err)

	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.PolicyReasonInvalid),
		Message:            fmt.Sprintf("%s: default/ca", backendtlspolicy.ErrConfigMapNotFound),
		ObservedGeneration: backendTlsPolicy.Generation,
	})
}

func (s *tsuite) assertPolicyStatus(inCondition metav1.Condition) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.TestInstallation.AssertionsT(s.T())
	p.Gomega.Eventually(func(g gomega.Gomega) {
		tlsPol := &gwv1.BackendTLSPolicy{}
		objKey := client.ObjectKeyFromObject(backendTlsPolicy)
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, tlsPol)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get BackendTLSPolicy %s", objKey)

		g.Expect(tlsPol.Status.Ancestors).To(gomega.HaveLen(2), "ancestors didn't have length of 2")

		expectedAncestorRefs := []gwv1.ParentReference{
			{
				Group:     (*gwv1.Group)(&svcGroup),
				Kind:      (*gwv1.Kind)(&svcKind),
				Namespace: ptr.To(gwv1.Namespace(nginxMeta.Namespace)),
				Name:      gwv1.ObjectName(nginxMeta.Name),
			},
			{
				Group:     (*gwv1.Group)(&svcGroup),
				Kind:      (*gwv1.Kind)(&svcKind),
				Namespace: ptr.To(gwv1.Namespace(nginx2Meta.Namespace)),
				Name:      gwv1.ObjectName(nginx2Meta.Name),
			},
		}

		for i, ancestor := range tlsPol.Status.Ancestors {
			expectedRef := expectedAncestorRefs[i]
			g.Expect(ancestor.AncestorRef).To(gomega.BeEquivalentTo(expectedRef))

			g.Expect(ancestor.Conditions).To(gomega.HaveLen(2), "ancestors conditions wasn't length of 2")
			cond := meta.FindStatusCondition(ancestor.Conditions, inCondition.Type)
			g.Expect(cond).NotTo(gomega.BeNil(), "policy should have accepted condition")
			g.Expect(cond.Status).To(gomega.Equal(inCondition.Status), "policy accepted condition should be true")
			g.Expect(cond.Reason).To(gomega.Equal(inCondition.Reason), "policy reason should be accepted")
			g.Expect(cond.Message).To(gomega.Equal(inCondition.Message))
			g.Expect(cond.ObservedGeneration).To(gomega.Equal(inCondition.ObservedGeneration))
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

const (
	kgatewayControllerName = "kgateway.dev/kgateway"
	otherControllerName    = "other-controller.example.com/controller"
)

// TestBackendTLSPolicyClearStaleStatus verifies that stale status is cleared when targetRef becomes invalid
func (s *tsuite) TestBackendTLSPolicyClearStaleStatus() {
	if s.agentgateway {
		s.T().Log("Skipping status test for Agentgateway as statuses are not currently supported")
		return
	}

	// Test applies base.yaml via setup which includes "tls-policy" targeting Services "nginx" and "nginx2"
	// Add fake ancestor status from another controller
	s.addAncestorStatus("tls-policy", "default", otherControllerName)

	// Verify both kgateway and other controller statuses exist
	s.assertAncestorStatuses("nginx", map[string]bool{
		kgatewayControllerName: true,
		otherControllerName:    true,
	})

	// Apply policy with missing service target
	err := s.TestInstallation.Actions.Kubectl().ApplyFile(
		s.Ctx,
		backendTLSPolicyMissingTargetManifest,
	)
	s.Require().NoError(err)

	// Verify kgateway status cleared, other remains
	s.assertAncestorStatuses("nginx", map[string]bool{
		kgatewayControllerName: false,
		otherControllerName:    true,
	})
	// AfterTest() handles cleanup automatically
}

func (s *tsuite) addAncestorStatus(policyName, policyNamespace, controllerName string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		policy := &gwv1.BackendTLSPolicy{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: policyName, Namespace: policyNamespace},
			policy,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Add fake ancestor status
		fakeStatus := gwv1.PolicyAncestorStatus{
			AncestorRef: gwv1.ParentReference{
				Group:     (*gwv1.Group)(&svcGroup),
				Kind:      (*gwv1.Kind)(&svcKind),
				Namespace: ptr.To(gwv1.Namespace(nginxMeta.Namespace)),
				Name:      gwv1.ObjectName(nginxMeta.Name),
			},
			ControllerName: gwv1.GatewayController(controllerName),
			Conditions: []metav1.Condition{
				{
					Type:               string(shared.PolicyConditionAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             string(shared.PolicyReasonValid),
					Message:            "Accepted by fake controller",
					LastTransitionTime: metav1.Now(),
				},
			},
		}

		policy.Status.Ancestors = append(policy.Status.Ancestors, fakeStatus)
		err = s.TestInstallation.ClusterContext.Client.Status().Update(s.Ctx, policy)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (s *tsuite) assertAncestorStatuses(ancestorName string, expectedControllers map[string]bool) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		policy := &gwv1.BackendTLSPolicy{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: "tls-policy", Namespace: "default"},
			policy,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		foundControllers := make(map[string]bool)
		for _, ancestor := range policy.Status.Ancestors {
			if string(ancestor.AncestorRef.Name) == ancestorName {
				foundControllers[string(ancestor.ControllerName)] = true
			}
		}

		for controller, shouldExist := range expectedControllers {
			exists := foundControllers[controller]
			if shouldExist {
				g.Expect(exists).To(gomega.BeTrue(), "Expected controller %s to exist in status", controller)
			} else {
				g.Expect(exists).To(gomega.BeFalse(), "Expected controller %s to not exist in status", controller)
			}
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

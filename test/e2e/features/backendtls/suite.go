//go:build e2e

package backendtls

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var (
	configMapManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata/configmap.yaml")
	backendTLSPolicyMissingTargetManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata/missing-target.yaml")

	backendTlsPolicy = &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-policy",
			Namespace: "kgateway-base",
		},
	}
	gatewayMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}
	gatewayGroup = gwv1.Group(gwv1.GroupVersion.Group)
	gatewayKind  = gwv1.Kind("Gateway")

	// base setup manifests
	baseSetupManifests = []string{
		filepath.Join(fsutils.MustGetThisDir(), "testdata/nginx.yaml"),
		configMapManifest,
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestBackendTLSPolicyAndStatus": {},
	}
)

type tsuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: append([]string{filepath.Join(fsutils.MustGetThisDir(), "testdata/base.yaml")}, baseSetupManifests...),
	}
	return &tsuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases, base.WithMinGwApiVersion(base.GwApiRequireBackendTLSPolicy)),
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
		common.BaseGateway.Send(
			s.T(),
			&matchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gomega.ContainSubstring(defaults.NginxResponse),
			},
			curl.WithPort(80),
			curl.WithHostHeader(tc.host),
			curl.WithPath("/"),
		)
	}

	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			// google returns 404 when going to google.com with host header of "foo.com"
			StatusCode: http.StatusNotFound,
		},
		curl.WithPort(80),
		curl.WithHostHeader("foo.com"),
		curl.WithPath("/"),
	)

	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwv1.PolicyReasonAccepted),
		Message:            reports.PolicyAcceptedMsg,
		ObservedGeneration: backendTlsPolicy.Generation,
	})
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.BackendTLSPolicyConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwv1.BackendTLSPolicyReasonResolvedRefs),
		Message:            "Resolved all references",
		ObservedGeneration: backendTlsPolicy.Generation,
	})

	// delete configmap so we can assert status updates correctly
	err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, configMapManifest)
	s.Require().NoError(err)

	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.BackendTLSPolicyConditionResolvedRefs),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.BackendTLSPolicyReasonInvalidCACertificateRef),
		Message:            fmt.Sprintf("invalid CA certificate ref ConfigMap/ca: %s: kgateway-base/ca", backendtlspolicy.ErrConfigMapNotFound),
		ObservedGeneration: backendTlsPolicy.Generation,
	})
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.BackendTLSPolicyReasonNoValidCACertificate),
		Message:            fmt.Sprintf("invalid CA certificate ref ConfigMap/ca: %s: kgateway-base/ca", backendtlspolicy.ErrConfigMapNotFound),
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

		g.Expect(tlsPol.Status.Ancestors).To(gomega.HaveLen(1), "ancestors didn't have length of 1")

		expectedRef := gwv1.ParentReference{
			Group:     &gatewayGroup,
			Kind:      &gatewayKind,
			Namespace: new(gwv1.Namespace(gatewayMeta.Namespace)),
			Name:      gwv1.ObjectName(gatewayMeta.Name),
		}
		ancestor := tlsPol.Status.Ancestors[0]
		g.Expect(ancestor.AncestorRef).To(gomega.BeEquivalentTo(expectedRef))

		g.Expect(ancestor.Conditions).To(gomega.HaveLen(2), "ancestor conditions wasn't length of 2")
		cond := meta.FindStatusCondition(ancestor.Conditions, inCondition.Type)
		g.Expect(cond).NotTo(gomega.BeNil(), "policy should have expected condition")
		g.Expect(cond.Status).To(gomega.Equal(inCondition.Status), "policy condition should have expected status")
		g.Expect(cond.Reason).To(gomega.Equal(inCondition.Reason), "policy reason should match")
		g.Expect(cond.Message).To(gomega.Equal(inCondition.Message))
		g.Expect(cond.ObservedGeneration).To(gomega.Equal(inCondition.ObservedGeneration))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

const (
	kgatewayControllerName = "kgateway.dev/kgateway"
	otherControllerName    = "other-controller.example.com/controller"
)

// TestBackendTLSPolicyClearStaleStatus verifies that stale status is cleared when targetRef becomes invalid
func (s *tsuite) TestBackendTLSPolicyClearStaleStatus() {
	// Test applies base.yaml via setup which includes "tls-policy" targeting Services "nginx" and "nginx2"
	// Add fake ancestor status from another controller
	s.addAncestorStatus("tls-policy", "kgateway-base", otherControllerName)

	// Verify both kgateway and other controller statuses exist
	s.assertAncestorStatuses("gateway", map[string]bool{
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
	s.assertAncestorStatuses("gateway", map[string]bool{
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
				Group:     &gatewayGroup,
				Kind:      &gatewayKind,
				Namespace: new(gwv1.Namespace(gatewayMeta.Namespace)),
				Name:      gwv1.ObjectName(gatewayMeta.Name),
			},
			ControllerName: gwv1.GatewayController(controllerName),
			Conditions: []metav1.Condition{
				{
					Type:               string(gwv1.PolicyConditionAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             string(gwv1.PolicyReasonAccepted),
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
			types.NamespacedName{Name: "tls-policy", Namespace: "kgateway-base"},
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

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
	kgwreports "github.com/kgateway-dev/kgateway/v2/pkg/reports"
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
	terminatedTLSRouteManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata/terminated-tlsroute.yaml")
	terminatedTLSRouteInvalidManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata/terminated-tlsroute-invalid.yaml")
	unroutedBackendManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata/unrouted-backend.yaml")

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
	wellknownBackendTlsPolicy = &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wellknown-tls-policy",
			Namespace: "kgateway-base",
		},
	}
	unroutedBackendTlsPolicy = &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrouted-tls-policy",
			Namespace: "kgateway-base",
		},
	}
	terminatedTLSRoutePolicy = &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tlsroute-backend-tls",
			Namespace: "kgateway-base",
		},
	}
	terminatedTLSRouteGatewayMeta = metav1.ObjectMeta{
		Name:      "tlsroute-gateway",
		Namespace: "kgateway-base",
	}
	terminatedTLSRouteInvalidPolicy = &gwv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-tlsroute-backend-tls",
			Namespace: "kgateway-base",
		},
	}
	terminatedTLSRouteInvalidGatewayMeta = metav1.ObjectMeta{
		Name:      "invalid-tlsroute-gateway",
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
		"TestBackendTLSPolicyErrorStatusForTerminatedTLSRoute": {
			Manifests:       []string{terminatedTLSRouteInvalidManifest},
			MinGwApiVersion: base.GwApiRequireTlsRoutes,
		},
		"TestBackendTLSPolicyStatusForTerminatedTLSRoute": {
			Manifests:       []string{terminatedTLSRouteManifest},
			MinGwApiVersion: base.GwApiRequireTlsRoutes,
		},
		"TestBackendTLSPolicyStatusForUnroutedBackend": {
			Manifests: []string{unroutedBackendManifest},
		},
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
		Message:            resolvedAllReferencesMsg,
		ObservedGeneration: backendTlsPolicy.Generation,
	})

	// A policy on a routed kgateway Backend reports both the Gateway and the Backend.
	s.assertPolicyStatusForPolicy(wellknownBackendTlsPolicy, gatewayMeta, metav1.Condition{
		Type:    string(gwv1.PolicyConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1.PolicyReasonAccepted),
		Message: reports.PolicyAcceptedMsg,
	})

	// delete configmap so we can assert status updates correctly
	err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, configMapManifest)
	s.Require().NoError(err)

	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.BackendTLSPolicyConditionResolvedRefs),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.BackendTLSPolicyReasonInvalidCACertificateRef),
		Message:            invalidCAConfigMapMessage("ca"),
		ObservedGeneration: backendTlsPolicy.Generation,
	})
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.BackendTLSPolicyReasonNoValidCACertificate),
		Message:            invalidCAConfigMapMessage("ca"),
		ObservedGeneration: backendTlsPolicy.Generation,
	})
}

func (s *tsuite) TestBackendTLSPolicyStatusForTerminatedTLSRoute() {
	s.assertPolicyStatusForPolicy(terminatedTLSRoutePolicy, terminatedTLSRouteGatewayMeta, metav1.Condition{
		Type:    string(gwv1.PolicyConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1.PolicyReasonAccepted),
		Message: reports.PolicyAcceptedMsg,
	})
	s.assertPolicyStatusForPolicy(terminatedTLSRoutePolicy, terminatedTLSRouteGatewayMeta, metav1.Condition{
		Type:    string(gwv1.BackendTLSPolicyConditionResolvedRefs),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1.BackendTLSPolicyReasonResolvedRefs),
		Message: resolvedAllReferencesMsg,
	})
}

func (s *tsuite) TestBackendTLSPolicyErrorStatusForTerminatedTLSRoute() {
	errMessage := invalidCAConfigMapMessage("missing-ca")
	s.assertPolicyStatusForPolicy(terminatedTLSRouteInvalidPolicy, terminatedTLSRouteInvalidGatewayMeta, metav1.Condition{
		Type:    string(gwv1.BackendTLSPolicyConditionResolvedRefs),
		Status:  metav1.ConditionFalse,
		Reason:  string(gwv1.BackendTLSPolicyReasonInvalidCACertificateRef),
		Message: errMessage,
	})
	s.assertPolicyStatusForPolicy(terminatedTLSRouteInvalidPolicy, terminatedTLSRouteInvalidGatewayMeta, metav1.Condition{
		Type:    string(gwv1.PolicyConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(gwv1.BackendTLSPolicyReasonNoValidCACertificate),
		Message: errMessage,
	})
}

// TestBackendTLSPolicyStatusForUnroutedBackend verifies that a policy on a Backend no route
// references still reports status, against the Backend itself, since kgateway applies the
// TLS config to that Backend's cluster regardless of routing (e.g. a GatewayExtension backend).
func (s *tsuite) TestBackendTLSPolicyStatusForUnroutedBackend() {
	s.assertPolicyStatusWithAncestors(unroutedBackendTlsPolicy, metav1.Condition{
		Type:    string(gwv1.PolicyConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1.PolicyReasonAccepted),
		Message: reports.PolicyAcceptedMsg,
	})
	s.assertPolicyStatusWithAncestors(unroutedBackendTlsPolicy, metav1.Condition{
		Type:    string(gwv1.BackendTLSPolicyConditionResolvedRefs),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1.BackendTLSPolicyReasonResolvedRefs),
		Message: resolvedAllReferencesMsg,
	})
}

func (s *tsuite) assertPolicyStatus(inCondition metav1.Condition) {
	s.assertPolicyStatusForPolicy(backendTlsPolicy, gatewayMeta, inCondition)
}

// assertPolicyStatusForPolicy asserts the policy is reported against the given Gateway, and
// against each of its targets, with the expected condition on every ancestor.
func (s *tsuite) assertPolicyStatusForPolicy(
	policy *gwv1.BackendTLSPolicy,
	ancestorMeta metav1.ObjectMeta,
	inCondition metav1.Condition,
) {
	s.assertPolicyStatusWithAncestors(policy, inCondition, gatewayParentReference(ancestorMeta))
}

// assertPolicyStatusWithAncestors asserts that kgateway owns exactly one ancestor per
// spec.targetRef plus each of extraAncestors, and that every one of them carries inCondition.
// Ancestors written by other controllers are ignored.
func (s *tsuite) assertPolicyStatusWithAncestors(
	policy *gwv1.BackendTLSPolicy,
	inCondition metav1.Condition,
	extraAncestors ...gwv1.ParentReference,
) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.TestInstallation.AssertionsT(s.T())
	p.Gomega.Eventually(func(g gomega.Gomega) {
		tlsPol := &gwv1.BackendTLSPolicy{}
		objKey := client.ObjectKeyFromObject(policy)
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, tlsPol)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get BackendTLSPolicy %s", objKey)

		expectedRefs := append(targetAncestorReferences(tlsPol), extraAncestors...)

		var ours []gwv1.PolicyAncestorStatus
		for _, ancestor := range tlsPol.Status.Ancestors {
			if string(ancestor.ControllerName) == kgatewayControllerName {
				ours = append(ours, ancestor)
			}
		}
		g.Expect(ours).To(gomega.HaveLen(len(expectedRefs)), "kgateway should own one ancestor per target plus %d extra, got %v", len(extraAncestors), ours)

		expectedObservedGeneration := inCondition.ObservedGeneration
		if expectedObservedGeneration == 0 {
			expectedObservedGeneration = tlsPol.Generation
		}
		for _, expectedRef := range expectedRefs {
			idx := -1
			for i, ancestor := range ours {
				if kgwreports.ParentRefEqual(ancestor.AncestorRef, expectedRef) {
					idx = i
					break
				}
			}
			g.Expect(idx).NotTo(gomega.Equal(-1), "missing kgateway ancestor %s in %v", kgwreports.ParentString(expectedRef), ours)
			ancestor := ours[idx]

			g.Expect(ancestor.Conditions).To(gomega.HaveLen(2), "ancestor %s conditions wasn't length of 2", kgwreports.ParentString(expectedRef))
			cond := meta.FindStatusCondition(ancestor.Conditions, inCondition.Type)
			g.Expect(cond).NotTo(gomega.BeNil(), "ancestor %s should have condition %s", kgwreports.ParentString(expectedRef), inCondition.Type)
			g.Expect(cond.Status).To(gomega.Equal(inCondition.Status), "policy condition should have expected status")
			g.Expect(cond.Reason).To(gomega.Equal(inCondition.Reason), "policy reason should match")
			g.Expect(cond.Message).To(gomega.Equal(inCondition.Message))
			g.Expect(cond.ObservedGeneration).To(gomega.Equal(expectedObservedGeneration))
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// targetAncestorReferences returns the ancestor refs kgateway reports for a policy's targets:
// the target itself, in the policy's namespace, with the targetRef's sectionName if any.
func targetAncestorReferences(policy *gwv1.BackendTLSPolicy) []gwv1.ParentReference {
	refs := make([]gwv1.ParentReference, 0, len(policy.Spec.TargetRefs))
	for _, target := range policy.Spec.TargetRefs {
		group := target.Group
		kind := target.Kind
		namespace := gwv1.Namespace(policy.Namespace)
		ref := gwv1.ParentReference{
			Group:     &group,
			Kind:      &kind,
			Namespace: &namespace,
			Name:      target.Name,
		}
		if target.SectionName != nil && *target.SectionName != "" {
			sectionName := *target.SectionName
			ref.SectionName = &sectionName
		}
		refs = append(refs, ref)
	}
	return refs
}

func gatewayParentReference(objMeta metav1.ObjectMeta) gwv1.ParentReference {
	namespace := gwv1.Namespace(objMeta.Namespace)
	return gwv1.ParentReference{
		Group:     &gatewayGroup,
		Kind:      &gatewayKind,
		Namespace: &namespace,
		Name:      gwv1.ObjectName(objMeta.Name),
	}
}

func invalidCAConfigMapMessage(name string) string {
	return fmt.Sprintf("invalid CA certificate ref ConfigMap/%s: %s: kgateway-base/%s", name, backendtlspolicy.ErrConfigMapNotFound, name)
}

const (
	resolvedAllReferencesMsg = "Resolved all references"
	kgatewayControllerName   = "kgateway.dev/kgateway"
	otherControllerName      = "other-controller.example.com/controller"
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
			AncestorRef:    gatewayParentReference(gatewayMeta),
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

package listenerset

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	if !RequiredCrdExists(s.TestInstallation) {
		s.T().Skip("Skipping as the XListenerSet CRD is not installed")
	}

	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestValidListenerSet() {
	s.expectListenerSetAccepted(validListenerSet)

	// Gateway Listener
	// The route attached to the gateway should work on the listener defined on the gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8080),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The route attached to the listener set should NOT work on the listener defined on the gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8080),
			curl.WithHostHeader("listenerset.com"),
		},
		expectNotFound)

	// Listener Set Listeners
	// The route attached to the gateway should work on the listener defined on the listener set
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8090),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The route attached to the listener set should work on the listener defined on the listener set
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8090),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOK)

	// The route attached to the listener set should not work on the listener it did not target
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8090),
			curl.WithHostHeader("listenerset-section.com"),
		},
		expectNotFound)

	// The route attached to the gateway should work on the listener defined on the listener set
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8091),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The route attached to the listener set should work on the listener defined on the listener set
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8091),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOK)

	// The route attached to the listener set should work on the listener it targets
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8091),
			curl.WithHostHeader("listenerset-section.com"),
		},
		expectOK)
}

func (s *testingSuite) TestInvalidListenerSetNotAllowed() {
	s.expectListenerSetNotAllowed(invalidListenerSetNotAllowed)

	// The route attached to the gateway should work on the listener defined on the gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8080),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The listener defined on the invalid listenerset should not work
	s.TestInstallation.Assertions.AssertEventualCurlError(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8090),
			curl.WithHostHeader("example.com"),
		},
		curlExitErrorCode)
}

func (s *testingSuite) TestInvalidListenerSetNonExistingGW() {
	s.expectListenerSetUnknown(invalidListenerSetNonExistingGW)

	// The route attached to the gateway should work on the listener defined on the gateway
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8080),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The listener defined on the invalid listenerset should not work
	s.TestInstallation.Assertions.AssertEventualCurlError(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8090),
			curl.WithHostHeader("example.com"),
		},
		curlExitErrorCode)
}

func (s *testingSuite) TestPolicies() {
	// The policy defined on the Gateway should apply to the Gateway listeners
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8080),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway"))

	// The policy defined on the Gateway should apply to the Gateway listeners it targets
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8081),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway-section"))

	// The policy defined on the Gateway should apply to the Listener Set listeners
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8095),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway"))

	// The policy defined on the Listener Set should apply to the Listener Set listeners
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8090),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "listener-set"))

	// The policy defined on the Listener Set should apply to the Listener Set listeners it targets
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(8091),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "listener-set-section"))
}

func (s *testingSuite) expectListenerSetAccepted(namespacedName types.NamespacedName) {
	s.TestInstallation.Assertions.EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.AttachedListenerSetsConditionType, metav1.ConditionTrue)

	s.TestInstallation.Assertions.EventuallyListenerSetStatus(s.Ctx, namespacedName.Name, namespacedName.Namespace,
		gwxv1a1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwxv1a1.ListenerSetConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gwxv1a1.ListenerSetReasonAccepted),
				},
				{
					Type:   string(gwxv1a1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionTrue,
					Reason: string(gwxv1a1.ListenerSetReasonProgrammed),
				},
			},
			Listeners: []gwxv1a1.ListenerEntryStatus{
				{
					Name:           "http",
					Port:           8090,
					AttachedRoutes: 2,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwxv1a1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwxv1a1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwxv1a1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwxv1a1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwxv1a1.ListenerEntryReasonResolvedRefs),
						},
						{
							Type:   string(gwxv1a1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwxv1a1.ListenerEntryReasonProgrammed),
						},
					},
				},
				{
					Name:           "http-2",
					Port:           8091,
					AttachedRoutes: 3,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwxv1a1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwxv1a1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwxv1a1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwxv1a1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwxv1a1.ListenerEntryReasonResolvedRefs),
						},
						{
							Type:   string(gwxv1a1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwxv1a1.ListenerEntryReasonProgrammed),
						},
					},
				},
			},
		})
}

func (s *testingSuite) expectListenerSetNotAllowed(namespacedName types.NamespacedName) {
	s.TestInstallation.Assertions.EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.AttachedListenerSetsConditionType, metav1.ConditionFalse)

	s.TestInstallation.Assertions.EventuallyListenerSetStatus(s.Ctx, namespacedName.Name, namespacedName.Namespace,
		gwxv1a1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwxv1a1.ListenerSetConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwxv1a1.ListenerSetReasonNotAllowed),
				},
				{
					Type:   string(gwxv1a1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwxv1a1.ListenerSetReasonNotAllowed),
				},
			},
		})
}

func (s *testingSuite) expectListenerSetUnknown(namespacedName types.NamespacedName) {
	s.TestInstallation.Assertions.EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.AttachedListenerSetsConditionType, metav1.ConditionFalse)

	s.TestInstallation.Assertions.EventuallyListenerSetStatus(s.Ctx, namespacedName.Name, namespacedName.Namespace,
		gwxv1a1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwxv1a1.ListenerSetConditionAccepted),
					Status: metav1.ConditionUnknown,
				},
				{
					Type:   string(gwxv1a1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionUnknown,
				},
			},
		})
}

func RequiredCrdExists(testInstallation *e2e.TestInstallation) bool {
	xListenerSetExists, err := schemes.CRDExists(testInstallation.ClusterContext.RestConfig, gwxv1a1.GroupVersion.Group, gwxv1a1.GroupVersion.Version, wellknown.XListenerSetKind)
	testInstallation.Assertions.Assert.NoError(err)
	return xListenerSetExists
}

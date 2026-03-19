//go:build e2e

package listenerset

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireListenerSets),
		),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestValidListenerSet() {
	s.expectValidListenerSetAccepted(validListenerSet)

	// Gateway Listener
	// The route attached to the gateway should work on the listener defined on the gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The route attached to the listener set should NOT work on the listener defined on the gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectNotFound)

	// Listener Set Listeners
	// The route attached to the gateway should NOT work on the listener defined on the listener set
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectNotFound)

	// The route attached to the listener set should work on the listener defined on the listener set
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOK)

	// The route attached to the listener set should not work on the section it did not target
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset-section.com"),
		},
		expectNotFound)

	// The route attached to the gateway should NOT work on the listener defined on the listener set
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener2Port),
			curl.WithHostHeader("example.com"),
		},
		expectNotFound)

	// The route attached to the listener set should work on the listener defined on the listener set
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener2Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOK)

	// The route attached to the listener set should work on the section it targets
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener2Port),
			curl.WithHostHeader("listenerset-section.com"),
		},
		expectOK)
}

func (s *testingSuite) TestInvalidListenerSetNotAllowed() {
	s.expectInvalidListenerSetNotAllowed(invalidListenerSetNotAllowed)

	// The route attached to the gateway should work on the listener defined on the gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The listener defined on the invalid listenerset should not work
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		curlExitErrorCode)
}

func (s *testingSuite) TestInvalidListenerSetNonExistingGW() {
	s.expectInvalidListenerSetUnknown(invalidListenerSetNonExistingGW)

	// The route attached to the gateway should work on the listener defined on the gateway
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The listener defined on the invalid listenerset should not work
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		curlExitErrorCode)
}

func (s *testingSuite) TestConflictedListenerSet() {
	s.expectGatewayAccepted(proxyService)
	s.expectValidListenerSetAccepted(validListenerSet)
	s.expectConflictedListenerSetConflicted(conflictedListenerSet)

	// The first listener with hostname conflict should work based on listener precedence
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectOK)

	// The other listener with hostname conflict should not work based on listener precedence
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("conflicted-listenerset.com"),
		},
		expectNotFound)

	// The first listener with protocol conflict should work based on listener precedence
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOK)

	// The other listener with protocol conflict should not work based on listener precedence
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("conflicted-listenerset.com"),
		},
		expectNotFound)

	// The listener without any conflict defined on the listenerset should work
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls3Listener1Port),
			curl.WithHostHeader("conflicted-listenerset.com"),
		},
		expectOK)
}

func (s *testingSuite) TestPolicies() {
	// The policy defined on the Gateway should apply to the Gateway listeners
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener1Port),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway"))

	// The policy defined on the Gateway should apply to the Gateway section it targets
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(gwListener2Port),
			curl.WithHostHeader("example.com"),
		},
		expectOKWithCustomHeader("policy", "gateway-section"))

	// The policy defined on the Listener Set should apply to the Listener Set listeners
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener1Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOKWithCustomHeader("policy", "listener-set"))

	// The policy defined on the Listener Set should apply to the Listener Set section it targets
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls1Listener2Port),
			curl.WithHostHeader("listenerset.com"),
		},
		expectOKWithCustomHeader("policy", "listener-set-section"))

	// TODO: Update this when we decide if policies should not be inherited
	// The policy defined on the Gateway should apply to the Listener Set listeners
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(ls2Listener1Port),
			curl.WithHostHeader("listenerset-2.com"),
		},
		expectOKWithCustomHeader("policy", "gateway"))
}

func (s *testingSuite) expectValidListenerSetAccepted(obj client.Object) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.GatewayConditionAttachedListenerSets, metav1.ConditionTrue)

	s.TestInstallation.AssertionsT(s.T()).EventuallyListenerSetStatus(s.Ctx, obj.GetName(), obj.GetNamespace(),
		gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerSetConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerSetReasonAccepted),
				},
				{
					Type:   string(gwv1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerSetReasonProgrammed),
				},
			},
			Listeners: []gwv1.ListenerEntryStatus{
				{
					Name:           "http",
					AttachedRoutes: 1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonProgrammed),
						},
					},
				},
				{
					Name:           "http-2",
					AttachedRoutes: 2,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonProgrammed),
						},
					},
				},
			},
		})
}

func (s *testingSuite) expectInvalidListenerSetNotAllowed(obj client.Object) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.GatewayConditionAttachedListenerSets, metav1.ConditionFalse)

	s.TestInstallation.AssertionsT(s.T()).EventuallyListenerSetStatus(s.Ctx, obj.GetName(), obj.GetNamespace(),
		gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerSetConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerSetReasonNotAllowed),
				},
				{
					Type:   string(gwv1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerSetReasonNotAllowed),
				},
			},
		})
}

func (s *testingSuite) expectInvalidListenerSetUnknown(obj client.Object) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.GatewayConditionAttachedListenerSets, metav1.ConditionFalse)

	s.TestInstallation.AssertionsT(s.T()).EventuallyListenerSetStatus(s.Ctx, obj.GetName(), obj.GetNamespace(),
		gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerSetConditionAccepted),
					Status: metav1.ConditionUnknown,
				},
				{
					Type:   string(gwv1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionUnknown,
				},
			},
		})
}

func (s *testingSuite) expectGatewayAccepted(obj client.Object) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayStatus(s.Ctx, obj.GetName(), obj.GetNamespace(),
		gwv1.GatewayStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.GatewayConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.GatewayReasonAccepted),
				},
				{
					Type:   string(gwv1.GatewayConditionProgrammed),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.GatewayReasonProgrammed),
				},
			},
			Listeners: []gwv1.ListenerStatus{
				{
					Name:           "http",
					AttachedRoutes: 1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonProgrammed),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
					},
				},
				{
					Name:           "http-2",
					AttachedRoutes: 1,
					Conditions: []metav1.Condition{
						// The first conflicted listener should be accepted based on listener precedence
						{
							Type:   string(gwv1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonProgrammed),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
					},
				},
			},
		})
}

func (s *testingSuite) expectConflictedListenerSetConflicted(obj client.Object) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace, listener.GatewayConditionAttachedListenerSets, metav1.ConditionTrue)

	s.TestInstallation.AssertionsT(s.T()).EventuallyListenerSetStatus(s.Ctx, obj.GetName(), obj.GetNamespace(),
		gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(gwv1.ListenerSetConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.GatewayReasonListenersNotValid),
				},
				{
					Type:   string(gwv1.ListenerSetConditionProgrammed),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.GatewayReasonListenersNotValid),
				},
			},
			Listeners: []gwv1.ListenerEntryStatus{
				{
					Name:           "gw-listener-hostname-conflict",
					AttachedRoutes: 1,
					Conditions: []metav1.Condition{
						{
							Type:    string(gwv1.ListenerEntryConditionAccepted),
							Status:  metav1.ConditionFalse,
							Reason:  string(gwv1.ListenerReasonHostnameConflict),
							Message: listener.ListenerMessageHostnameConflict,
						},
						{
							Type:    string(gwv1.ListenerEntryConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  string(gwv1.ListenerReasonHostnameConflict),
							Message: listener.ListenerMessageHostnameConflict,
						},
						{
							Type:    string(gwv1.ListenerEntryConditionConflicted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gwv1.ListenerReasonHostnameConflict),
							Message: listener.ListenerMessageHostnameConflict,
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
					},
				},
				{
					Name:           "ls-listener-protocol-conflict",
					AttachedRoutes: 0,
					Conditions: []metav1.Condition{
						{
							Type:    string(gwv1.ListenerEntryConditionAccepted),
							Status:  metav1.ConditionFalse,
							Reason:  string(gwv1.ListenerReasonProtocolConflict),
							Message: listener.ListenerMessageProtocolConflict,
						},
						{
							Type:    string(gwv1.ListenerEntryConditionProgrammed),
							Status:  metav1.ConditionFalse,
							Reason:  string(gwv1.ListenerReasonProtocolConflict),
							Message: listener.ListenerMessageProtocolConflict,
						},
						{
							Type:    string(gwv1.ListenerEntryConditionConflicted),
							Status:  metav1.ConditionTrue,
							Reason:  string(gwv1.ListenerReasonProtocolConflict),
							Message: listener.ListenerMessageProtocolConflict,
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
					},
				},
				{
					Name:           "http",
					AttachedRoutes: 1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.ListenerEntryConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonAccepted),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionConflicted),
							Status: metav1.ConditionFalse,
							Reason: string(gwv1.ListenerReasonNoConflicts),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionResolvedRefs),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonResolvedRefs),
						},
						{
							Type:   string(gwv1.ListenerEntryConditionProgrammed),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.ListenerEntryReasonProgrammed),
						},
					},
				},
			},
		})
}

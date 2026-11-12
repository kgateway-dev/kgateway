//go:build e2e

package listenerset

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
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

func (s *testingSuite) TestPolicies() {
	gw := common.NewGateway(s.Ctx, s.T(), s.TestInstallation, types.NamespacedName{
		Name:      proxyObjectMeta.Name,
		Namespace: proxyObjectMeta.Namespace,
	})

	// The policy defined on the Gateway should apply to the Gateway listeners
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "gateway"),
		curl.WithPort(gwListener1Port),
		curl.WithHostHeader("example.com"),
	)

	// The policy defined on the Gateway should apply to the Gateway section it targets
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "gateway-section"),
		curl.WithPort(gwListener2Port),
		curl.WithHostHeader("example.com"),
	)

	// The policy defined on the Listener Set should apply to the Listener Set listeners
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "listener-set"),
		curl.WithPort(ls1Listener1Port),
		curl.WithHostHeader("listenerset.com"),
	)

	// The policy defined on the Listener Set should apply to the Listener Set section it targets
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "listener-set-section"),
		curl.WithPort(ls1Listener2Port),
		curl.WithHostHeader("listenerset.com"),
	)

	// TODO: Update this when we decide if policies should not be inherited
	// The policy defined on the Gateway should apply to the Listener Set listeners
	gw.Send(
		s.T(),
		expectOKWithCustomHeader("policy", "gateway"),
		curl.WithPort(ls2Listener1Port),
		curl.WithHostHeader("listenerset-2.com"),
	)

	// denied-ls is in a namespace the Gateway's allowedListeners selector does not match.
	s.TestInstallation.AssertionsT(s.T()).EventuallyListenerSetStatus(s.Ctx, "denied-ls", "default", gwv1.ListenerSetStatus{
		Conditions: []metav1.Condition{{
			Type:   string(gwv1.ListenerSetConditionAccepted),
			Status: metav1.ConditionFalse,
			Reason: string(gwv1.ListenerSetReasonNotAllowed),
		}},
	})
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		svc := &corev1.Service{}
		g.Expect(s.TestInstallation.ClusterContext.Client.Get(s.Ctx, types.NamespacedName{Name: proxyObjectMeta.Name, Namespace: proxyObjectMeta.Namespace}, svc)).To(gomega.Succeed())
		g.Expect(slices.Map(svc.Spec.Ports, func(p corev1.ServicePort) int { return int(p.Port) })).To(gomega.ConsistOf(
			gwListener1Port, gwListener2Port, ls1Listener1Port, ls1Listener2Port, ls2Listener1Port,
		), "the proxy Service exposes the Gateway's and the allowed ListenerSets' ports only")
	}, "30s", "1s").Should(gomega.Succeed())
}

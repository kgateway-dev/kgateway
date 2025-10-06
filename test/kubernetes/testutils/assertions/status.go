package assertions

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

// EventuallyHTTPRouteStatusContainsMessage asserts that eventually at least one of the HTTPRoute's route parent statuses contains
// the given message substring.
func (p *Provider) EventuallyHTTPRouteStatusContainsMessage(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	message string,
	timeout ...time.Duration) {
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		matcher := matchers.HaveKubeGatewayRouteStatus(&matchers.KubeGatewayRouteStatus{
			Custom: gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Parents": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Conditions": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": matchers.ContainSubstrings([]string{message}),
					})),
				})),
			}),
		})

		route := &gwv1.HTTPRoute{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: routeName, Namespace: routeNamespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "can get httproute")
		g.Expect(route.Status.RouteStatus).To(gomega.HaveValue(matcher), fmt.Sprintf("Full status: %+v", route.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyHTTPRouteStatusContainsReason asserts that eventually at least one of the HTTPRoute's route parent statuses contains
// the given reason substring.
func (p *Provider) EventuallyHTTPRouteStatusContainsReason(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	reason string,
	timeout ...time.Duration,
) {
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		matcher := matchers.HaveKubeGatewayRouteStatus(&matchers.KubeGatewayRouteStatus{
			Custom: gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Parents": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Conditions": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Reason": matchers.ContainSubstrings([]string{reason}),
					})),
				})),
			}),
		})

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeName,
				Namespace: routeNamespace,
			},
		}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: routeName, Namespace: routeNamespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "can get httproute")
		g.Expect(route.Status.RouteStatus).To(gomega.HaveValue(matcher), fmt.Sprintf("Full status: %+v", route.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyGatewayCondition checks the provided Gateway condition is set to expect.
func (p *Provider) EventuallyGatewayCondition(
	ctx context.Context,
	gatewayName string,
	gatewayNamespace string,
	cond gwv1.GatewayConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		gateway := &gwv1.Gateway{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: gatewayNamespace}, gateway)
		g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("failed to get Gateway %s/%s", gatewayNamespace, gatewayName))

		condition := getConditionByType(gateway.Status.Conditions, string(cond))
		g.Expect(condition).NotTo(gomega.BeNil(), fmt.Sprintf("%v condition not found for Gateway %s/%s. Full status: %+v", cond, gatewayNamespace, gatewayName, gateway.Status))
		g.Expect(condition.Status).To(gomega.Equal(expect), fmt.Sprintf("%v condition is not %v for Gateway %s/%s. Full status: %+v",
			cond, expect, gatewayNamespace, gatewayName, gateway.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyGatewayListenerAttachedRoutes checks the provided Gateway contains the expected attached routes for the listener.
func (p *Provider) EventuallyGatewayListenerAttachedRoutes(
	ctx context.Context,
	gatewayName string,
	gatewayNamespace string,
	listener gwv1.SectionName,
	routes int32,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		gateway := &gwv1.Gateway{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: gatewayNamespace}, gateway)
		g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("failed to get Gateway %s/%s", gatewayNamespace, gatewayName))

		found := false
		for _, l := range gateway.Status.Listeners {
			if l.Name == listener {
				found = true
				g.Expect(l.AttachedRoutes).To(gomega.Equal(routes), fmt.Sprintf("%v listener does not contain %d attached routes for Gateway %s/%s. Full status: %+v",
					l, routes, gatewayNamespace, gatewayName, gateway.Status))
			}
		}
		g.Expect(found).To(gomega.BeTrue(), fmt.Sprintf("%v listener not found for Gateway %s/%s. Full status: %+v", listener, gatewayNamespace, gatewayName, gateway.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (p *Provider) EventuallyGatewayStatus(
	ctx context.Context,
	name string,
	namespace string,
	status gwv1.GatewayStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		gw := &gwv1.Gateway{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, gw)
		g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("failed to get gateway %s/%s", namespace, name))

		for _, expected := range status.Conditions {
			condition := getConditionByType(gw.Status.Conditions, expected.Type)
			g.Expect(condition).NotTo(gomega.BeNil(), fmt.Sprintf("%v condition not found for gateway %s/%s. Full status: %+v", expected.Type, namespace, name, gw.Status))
			g.Expect(condition.Status).To(gomega.Equal(expected.Status), fmt.Sprintf("%v status is not %v for gateway %s/%s. Full status: %+v", expected, expected.Status, namespace, name, gw.Status))
			if expected.Reason != "" {
				g.Expect(condition.Reason).To(gomega.Equal(expected.Reason), fmt.Sprintf("%v reason is not %v for gateway %s/%s. Full status: %+v", expected, expected.Reason, namespace, name, gw.Status))
			}
		}

		for _, expectedListener := range status.Listeners {
			listenerStatus := getListenerStatus(gw.Status.Listeners, string(expectedListener.Name))
			g.Expect(listenerStatus).NotTo(gomega.BeNil(), fmt.Sprintf("%v listener status not found for listener %s. Full status: %+v", expectedListener.Name, expectedListener.Name, gw.Status))
			if expectedListener.AttachedRoutes != 0 {
				g.Expect(listenerStatus.AttachedRoutes).To(gomega.Equal(expectedListener.AttachedRoutes), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expectedListener, expectedListener.AttachedRoutes, expectedListener.Name, gw.Status))
			}
			if expectedListener.SupportedKinds != nil {
				g.Expect(listenerStatus.SupportedKinds).To(gomega.ContainElements(expectedListener.SupportedKinds), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expectedListener, expectedListener.SupportedKinds, expectedListener.Name, gw.Status))
			}

			for _, expected := range expectedListener.Conditions {
				condition := getConditionByType(listenerStatus.Conditions, expected.Type)
				g.Expect(condition).NotTo(gomega.BeNil(), fmt.Sprintf("%v condition not found for listener %s. Full status: %+v", expected, expectedListener.Name, gw.Status))
				g.Expect(condition.Status).To(gomega.Equal(expected.Status), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expected, expected.Status, expectedListener.Name, gw.Status))
				if expected.Reason != "" {
					g.Expect(condition.Reason).To(gomega.Equal(expected.Reason), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expected, expected.Reason, expectedListener.Name, gw.Status))
				}
			}
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyHTTPRouteCondition checks that provided HTTPRoute condition is set to expect.
func (p *Provider) EventuallyHTTPRouteCondition(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	cond gwv1.RouteConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: routeName, Namespace: routeNamespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get HTTPRoute %s/%s", routeNamespace, routeName)

		var conditionFound bool
		for _, parentStatus := range route.Status.Parents {
			condition := getConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(), fmt.Sprintf("%v condition is not %v for any parent of HTTPRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyTCPRouteCondition checks that provided TCPRoute condition is set to expect.
func (p *Provider) EventuallyTCPRouteCondition(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	cond gwv1.RouteConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1a2.TCPRoute{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: routeName, Namespace: routeNamespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get TCPRoute %s/%s", routeNamespace, routeName)

		var conditionFound bool
		for _, parentStatus := range route.Status.Parents {
			condition := getConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(), fmt.Sprintf("%v condition is not %v for any parent of TCPRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyTLSRouteCondition checks that provided TLSRoute condition is set to expect.
func (p *Provider) EventuallyTLSRouteCondition(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	cond gwv1.RouteConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1a2.TLSRoute{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: routeName, Namespace: routeNamespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get TLSRoute %s/%s", routeNamespace, routeName)

		var conditionFound bool
		for _, parentStatus := range route.Status.Parents {
			condition := getConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(), fmt.Sprintf("%v condition is not %v for any parent of TLSRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyGRPCRouteCondition checks that provided GRPCRoute condition is set to expect.
func (p *Provider) EventuallyGRPCRouteCondition(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	cond gwv1.RouteConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.GRPCRoute{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: routeName, Namespace: routeNamespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get GRPCRoute %s/%s", routeNamespace, routeName)

		var conditionFound bool
		for _, parentStatus := range route.Status.Parents {
			condition := getConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(), fmt.Sprintf("%v condition is not %v for any parent of GRPCRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyInferencePoolCondition checks that the specified InferencePool condition
// eventually has the desired status on any parent managed by Kgateway.
func (p *Provider) EventuallyInferencePoolCondition(
	ctx context.Context,
	poolName string,
	poolNamespace string,
	cond inf.InferencePoolConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()

	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		pool := &inf.InferencePool{}
		err := p.clusterContext.Client.Get(
			ctx,
			types.NamespacedName{Name: poolName, Namespace: poolNamespace},
			pool,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred(),
			"failed to get InferencePool %s/%s", poolNamespace, poolName)

		var conditionFound bool
		for _, parent := range pool.Status.Parents {
			// Look for the first matching condition on any parent.
			if c := getConditionByType(parent.Conditions, string(cond)); c != nil && c.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(),
			fmt.Sprintf("%v condition is not %v for any parent of InferencePool %s/%s",
				cond, expect, poolNamespace, poolName))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// Helper function to retrieve a condition by type from a list of conditions.
func getConditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func (p *Provider) EventuallyListenerSetStatus(
	ctx context.Context,
	name string,
	namespace string,
	status gwxv1a1.ListenerSetStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		ls := &gwxv1a1.XListenerSet{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ls)
		g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("failed to get listenerset %s/%s", namespace, name))

		for _, expected := range status.Conditions {
			condition := getConditionByType(ls.Status.Conditions, expected.Type)
			g.Expect(condition).NotTo(gomega.BeNil(), fmt.Sprintf("%v condition not found for listenerset %s/%s. Full status: %+v", expected.Type, namespace, name, ls.Status))
			g.Expect(condition.Status).To(gomega.Equal(expected.Status), fmt.Sprintf("%v status is not %v for listenerset %s/%s. Full status: %+v", expected, expected.Status, namespace, name, ls.Status))
			if expected.Reason != "" {
				g.Expect(condition.Reason).To(gomega.Equal(expected.Reason), fmt.Sprintf("%v reason is not %v for listenerset %s/%s. Full status: %+v", expected, expected.Reason, namespace, name, ls.Status))
			}
		}

		for _, expectedListener := range status.Listeners {
			listenerStatus := getListenerEntryStatus(ls.Status.Listeners, string(expectedListener.Name))
			g.Expect(listenerStatus).NotTo(gomega.BeNil(), fmt.Sprintf("%v listener status not found for listener %s. Full status: %+v", expectedListener.Name, expectedListener.Name, ls.Status))
			if expectedListener.Port != 0 {
				g.Expect(listenerStatus.Port).To(gomega.Equal(expectedListener.Port), fmt.Sprintf("%v listener condition is not %v for listener %s. Full status: %+v", expectedListener, expectedListener.Port, expectedListener.Name, ls.Status))
			}
			if expectedListener.AttachedRoutes != 0 {
				g.Expect(listenerStatus.AttachedRoutes).To(gomega.Equal(expectedListener.AttachedRoutes), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expectedListener, expectedListener.AttachedRoutes, expectedListener.Name, ls.Status))
			}
			if expectedListener.SupportedKinds != nil {
				g.Expect(listenerStatus.SupportedKinds).To(gomega.ContainElements(expectedListener.SupportedKinds), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expectedListener, expectedListener.SupportedKinds, expectedListener.Name, ls.Status))
			}

			for _, expected := range expectedListener.Conditions {
				condition := getConditionByType(listenerStatus.Conditions, expected.Type)
				g.Expect(condition).NotTo(gomega.BeNil(), fmt.Sprintf("%v condition not found for listener %s. Full status: %+v", expected, expectedListener.Name, ls.Status))
				g.Expect(condition.Status).To(gomega.Equal(expected.Status), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expected, expected.Status, expectedListener.Name, ls.Status))
				if expected.Reason != "" {
					g.Expect(condition.Reason).To(gomega.Equal(expected.Reason), fmt.Sprintf("%v condition is not %v for listener %s. Full status: %+v", expected, expected.Reason, expectedListener.Name, ls.Status))
				}
			}
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (p *Provider) EventuallyListenerSetAttachedRoutes(
	ctx context.Context,
	name string,
	namespace string,
	listener gwv1.SectionName,
	routes int32,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		ls := &gwxv1a1.XListenerSet{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ls)
		g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("failed to get listenerset %s/%s", namespace, name))

		for _, expectedListener := range ls.Status.Listeners {
			listenerStatus := getListenerEntryStatus(ls.Status.Listeners, string(expectedListener.Name))
			g.Expect(listenerStatus).NotTo(gomega.BeNil(), fmt.Sprintf("%v listener status not found for listener %s. Full status: %+v", expectedListener.Name, expectedListener.Name, ls.Status))
			g.Expect(listenerStatus.AttachedRoutes).To(gomega.Equal(expectedListener.AttachedRoutes), fmt.Sprintf("%v AttachedRoutes is not %v for listener %s. Full status: %+v", expectedListener, expectedListener.AttachedRoutes, expectedListener.Name, ls.Status))
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func getListenerEntryStatus(listeners []gwxv1a1.ListenerEntryStatus, name string) *gwxv1a1.ListenerEntryStatus {
	for _, listener := range listeners {
		if string(listener.Name) == name {
			return &listener
		}
	}
	return nil
}

func getListenerStatus(listeners []gwv1.ListenerStatus, name string) *gwv1.ListenerStatus {
	for _, listener := range listeners {
		if string(listener.Name) == name {
			return &listener
		}
	}
	return nil
}

// EventuallyHTTPListenerPolicyCondition checks that provided HTTPListenerPolicy condition is set to expect.
func (p *Provider) EventuallyHTTPListenerPolicyCondition(
	ctx context.Context,
	name string,
	namespace string,
	cond gwv1.GatewayConditionType,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		hlp := &v1alpha1.HTTPListenerPolicy{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hlp)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get HTTPListenerPolicy %s/%s", namespace, name)

		var conditionFound bool
		for _, parentStatus := range hlp.Status.Ancestors {
			condition := getConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(), fmt.Sprintf("%v condition is not %v for any parent of HTTPListenerPolicy %s/%s. Full status: %+v",
			cond, expect, namespace, name, hlp.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

// EventuallyBackendCondition checks that provided Backend condition is set to expect.
func (p *Provider) EventuallyBackendCondition(
	ctx context.Context,
	name string,
	namespace string,
	condition string,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		backend := &v1alpha1.Backend{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, backend)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get Backend %s/%s", namespace, name)

		var conditionFound bool
		for _, cond := range backend.Status.Conditions {
			if cond.Type == condition && cond.Status == expect {
				conditionFound = true
				break
			}
		}
		g.Expect(conditionFound).To(gomega.BeTrue(), fmt.Sprintf("%v condition is not %v for Backend %s/%s. Full status: %+v",
			condition, expect, namespace, name, backend.Status))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

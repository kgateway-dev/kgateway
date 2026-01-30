//go:build e2e

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

// EventuallyGatewayAddress asserts that eventually at least one of the HTTPRoute's route parent statuses contains
// the given message substring.
func (p *Provider) EventuallyGatewayAddress(
	ctx context.Context,
	gatewayName string,
	gatewayNamespace string,
	timeout ...time.Duration,
) string {
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	var addr string
	p.Gomega.Eventually(func(g gomega.Gomega) {
		gw := &gwv1.Gateway{}
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: gatewayName, Namespace: gatewayNamespace}, gw)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "can get gateway")
		if len(gw.Status.Addresses) == 0 {
			g.Expect(true).To(gomega.BeFalse(), "gateway is not ready")
		}
		addr = gw.Status.Addresses[0].Value
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
	return addr
}

// EventuallyHTTPRouteStatusContainsMessage asserts that eventually at least one of the HTTPRoute's route parent statuses contains
// the given message substring.
func (p *Provider) EventuallyHTTPRouteStatusContainsMessage(
	ctx context.Context,
	routeName string,
	routeNamespace string,
	message string,
	timeout ...time.Duration,
) {
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
	eventuallyCondition(p, ctx, gatewayName, gatewayNamespace, &gwv1.Gateway{}, func(gw *gwv1.Gateway) bool {
		condition := GetConditionByType(gw.Status.Conditions, string(cond))
		return condition != nil && condition.Status == expect
	}, func(gw *gwv1.Gateway) string {
		return fmt.Sprintf("%v condition is not %v for Gateway %s/%s. Full status: %+v",
			cond, expect, gatewayNamespace, gatewayName, gw.Status)
	}, timeout...)
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
	eventuallyCondition(p, ctx, gatewayName, gatewayNamespace, &gwv1.Gateway{}, func(gw *gwv1.Gateway) bool {
		for _, l := range gw.Status.Listeners {
			if l.Name == listener {
				return l.AttachedRoutes == routes
			}
		}
		return false
	}, func(gw *gwv1.Gateway) string {
		return fmt.Sprintf("%v listener does not contain %d attached routes for Gateway %s/%s. Full status: %+v",
			listener, routes, gatewayNamespace, gatewayName, gw.Status)
	}, timeout...)
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
			condition := GetConditionByType(gw.Status.Conditions, expected.Type)
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
				condition := GetConditionByType(listenerStatus.Conditions, expected.Type)
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
	eventuallyCondition(p, ctx, routeName, routeNamespace, &gwv1.HTTPRoute{}, func(route *gwv1.HTTPRoute) bool {
		for _, parentStatus := range route.Status.Parents {
			condition := GetConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				return true
			}
		}
		return false
	}, func(route *gwv1.HTTPRoute) string {
		return fmt.Sprintf("%v condition is not %v for any parent of HTTPRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status)
	}, timeout...)
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
	eventuallyCondition(p, ctx, routeName, routeNamespace, &gwv1a2.TCPRoute{}, func(route *gwv1a2.TCPRoute) bool {
		for _, parentStatus := range route.Status.Parents {
			condition := GetConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				return true
			}
		}
		return false
	}, func(route *gwv1a2.TCPRoute) string {
		return fmt.Sprintf("%v condition is not %v for any parent of TCPRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status)
	}, timeout...)
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
	eventuallyCondition(p, ctx, routeName, routeNamespace, &gwv1a2.TLSRoute{}, func(route *gwv1a2.TLSRoute) bool {
		for _, parentStatus := range route.Status.Parents {
			condition := GetConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				return true
			}
		}
		return false
	}, func(route *gwv1a2.TLSRoute) string {
		return fmt.Sprintf("%v condition is not %v for any parent of TLSRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status)
	}, timeout...)
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
	eventuallyCondition(p, ctx, routeName, routeNamespace, &gwv1.GRPCRoute{}, func(route *gwv1.GRPCRoute) bool {
		for _, parentStatus := range route.Status.Parents {
			condition := GetConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				return true
			}
		}
		return false
	}, func(route *gwv1.GRPCRoute) string {
		return fmt.Sprintf("%v condition is not %v for any parent of GRPCRoute %s/%s. Full status: %+v",
			cond, expect, routeNamespace, routeName, route.Status)
	}, timeout...)
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

	ginkgo.GinkgoHelper()
	eventuallyCondition(p, ctx, poolName, poolNamespace, &inf.InferencePool{}, func(pool *inf.InferencePool) bool {
		for _, parent := range pool.Status.Parents {
			if c := GetConditionByType(parent.Conditions, string(cond)); c != nil && c.Status == expect {
				return true
			}
		}
		return false
	}, func(pool *inf.InferencePool) string {
		return fmt.Sprintf("%v condition is not %v for any parent of InferencePool %s/%s",
			cond, expect, poolNamespace, poolName)
	}, timeout...)
}

// Helper function to retrieve a condition by type from a list of conditions.
func GetConditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
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
			condition := GetConditionByType(ls.Status.Conditions, expected.Type)
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
				condition := GetConditionByType(listenerStatus.Conditions, expected.Type)
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
	eventuallyCondition(p, ctx, name, namespace, &kgateway.HTTPListenerPolicy{}, func(hlp *kgateway.HTTPListenerPolicy) bool {
		for _, parentStatus := range hlp.Status.Ancestors {
			condition := GetConditionByType(parentStatus.Conditions, string(cond))
			if condition != nil && condition.Status == expect {
				return true
			}
		}
		return false
	}, func(hlp *kgateway.HTTPListenerPolicy) string {
		return fmt.Sprintf("%v condition is not %v for any parent of HTTPListenerPolicy %s/%s. Full status: %+v",
			cond, expect, namespace, name, hlp.Status)
	}, timeout...)
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
	eventuallyCondition(p, ctx, name, namespace, &kgateway.Backend{}, func(backend *kgateway.Backend) bool {
		for _, cond := range backend.Status.Conditions {
			if cond.Type == condition && cond.Status == expect {
				return true
			}
		}
		return false
	}, func(backend *kgateway.Backend) string {
		return fmt.Sprintf("%v condition is not %v for Backend %s/%s. Full status: %+v",
			condition, expect, namespace, name, backend.Status)
	}, timeout...)
}

// EventuallyAgwBackendCondition checks that provided AgentgatewayBackend condition is set to expect.
func (p *Provider) EventuallyAgwBackendCondition(
	ctx context.Context,
	name string,
	namespace string,
	condition string,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	eventuallyCondition(p, ctx, name, namespace, &agentgateway.AgentgatewayBackend{}, func(backend *agentgateway.AgentgatewayBackend) bool {
		for _, cond := range backend.Status.Conditions {
			if cond.Type == condition && cond.Status == expect {
				return true
			}
		}
		return false
	}, func(backend *agentgateway.AgentgatewayBackend) string {
		return fmt.Sprintf("%v condition is not %v for AgentgatewayBackend %s/%s. Full status: %+v",
			condition, expect, namespace, name, backend.Status)
	}, timeout...)
}

// EventuallyAgwPolicyCondition checks that provided AgentgatewayPolicy condition is set to expect.
func (p *Provider) EventuallyAgwPolicyCondition(
	ctx context.Context,
	name string,
	namespace string,
	condType string,
	expect metav1.ConditionStatus,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	eventuallyCondition(p, ctx, name, namespace, &agentgateway.AgentgatewayPolicy{}, func(policy *agentgateway.AgentgatewayPolicy) bool {
		for _, parentStatus := range policy.Status.Ancestors {
			condition := GetConditionByType(parentStatus.Conditions, condType)
			if condition != nil && condition.Status == expect {
				return true
			}
		}
		return false
	}, func(policy *agentgateway.AgentgatewayPolicy) string {
		return fmt.Sprintf("%v condition is not %v for any ancestor of AgentgatewayPolicy %s/%s. Full status: %+v",
			condType, expect, namespace, name, policy.Status)
	}, timeout...)
}

// eventuallyCondition is a generic helper for checking status conditions on Kubernetes objects.
//
// Note on object reuse:
// This function reuses the object pointer `obj` for each `Client.Get` call within the generic `Eventually` loop.
// Client.Get overwrites the object. Since T is a pointer type (e.g. *gwv1.Gateway), passing the SAME pointer repeatedly
// to Get is fine as long as we don't need to accumulate state or worry about partial overwrites (Get usually overwrites).
// This reuse is standard practice in these tests.
func eventuallyCondition[T client.Object](
	p *Provider,
	ctx context.Context,
	name, namespace string,
	obj T,
	check func(T) bool,
	errMsg func(T) string,
	timeout ...time.Duration,
) {
	ginkgo.GinkgoHelper()
	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		err := p.clusterContext.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
		g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("failed to get %T %s/%s", obj, namespace, name))

		g.Expect(check(obj)).To(gomega.BeTrue(), errMsg(obj))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

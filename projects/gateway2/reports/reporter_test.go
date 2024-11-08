package reports_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/solo-io/gloo/projects/gateway2/reports"
)

var _ = Describe("Reporting Infrastructure", func() {
	BeforeEach(func() {
	})

	Describe("building gateway status", func() {
		It("should build all positive conditions with an empty report", func() {
			gw := gw()
			rm := reports.NewReportMap()

			reporter := reports.NewReporter(&rm)
			// initialize GatewayReporter to mimic translation loop (i.e. report gets initialized for all GWs)
			reporter.Gateway(gw)

			status := rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))
		})

		It("should correctly set negative gateway conditions from report and not add extra conditions", func() {
			gw := gw()
			rm := reports.NewReportMap()
			reporter := reports.NewReporter(&rm)
			reporter.Gateway(gw).SetCondition(reports.GatewayCondition{
				Type:   gwv1.GatewayConditionProgrammed,
				Status: metav1.ConditionFalse,
				Reason: gwv1.GatewayReasonAddressNotUsable,
			})
			status := rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))

			programmed := meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionProgrammed))
			Expect(programmed.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should correctly set negative listener conditions from report and not add extra conditions", func() {
			gw := gw()
			rm := reports.NewReportMap()
			reporter := reports.NewReporter(&rm)
			reporter.Gateway(gw).Listener(listener()).SetCondition(reports.ListenerCondition{
				Type:   gwv1.ListenerConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.ListenerReasonInvalidRouteKinds,
			})
			status := rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))

			resolvedRefs := meta.FindStatusCondition(status.Listeners[0].Conditions, string(gwv1.ListenerConditionResolvedRefs))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should not modify LastTransitionTime for existing conditions that have not changed", func() {
			gw := gw()
			rm := reports.NewReportMap()

			reporter := reports.NewReporter(&rm)
			// initialize GatewayReporter to mimic translation loop (i.e. report gets initialized for all GWs)
			reporter.Gateway(gw)

			status := rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))

			acceptedCond := meta.FindStatusCondition(status.Listeners[0].Conditions, string(gwv1.ListenerConditionAccepted))
			oldTransitionTime := acceptedCond.LastTransitionTime

			gw.Status = *status
			status = rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))

			acceptedCond = meta.FindStatusCondition(status.Listeners[0].Conditions, string(gwv1.ListenerConditionAccepted))
			newTransitionTime := acceptedCond.LastTransitionTime
			Expect(newTransitionTime).To(Equal(oldTransitionTime))
		})

		// TODO(Law): add multiple gws/listener tests
		// TODO(Law): add test confirming transitionTime change when status change
	})

	Describe("building route status", func() {
		DescribeTable("should build all positive route conditions with an empty report",
			func(obj client.Object) {
				rm := reports.NewReportMap()

				reporter := reports.NewReporter(&rm)
				// initialize RouteReporter to mimic translation loop (i.e. report gets initialized for all Routes)
				reporter.Route(obj)

				status := rm.BuildRouteStatus(context.Background(), obj, "gloo-gateway")

				Expect(status).NotTo(BeNil())
				Expect(status.Parents).To(HaveLen(1))
				Expect(status.Parents[0].Conditions).To(HaveLen(2))
			},
			Entry("regular httproute", httpRoute()),
			Entry("regular tcproute", tcpRoute()),
			Entry("delegatee route", delegateeRoute()),
		)

		DescribeTable("should correctly set negative route conditions from report and not add extra conditions",
			func(obj client.Object, parentRef *gwv1.ParentReference) {
				rm := reports.NewReportMap()
				reporter := reports.NewReporter(&rm)
				reporter.Route(obj).ParentRef(parentRef).SetCondition(reports.RouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})

				status := rm.BuildRouteStatus(context.Background(), obj, "gloo-gateway")

				Expect(status).NotTo(BeNil())
				Expect(status.Parents).To(HaveLen(1))
				Expect(status.Parents[0].Conditions).To(HaveLen(2))

				resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
			},
			Entry("regular httproute", httpRoute(), parentRef()),
			Entry("regular tcproute", tcpRoute(), parentRef()),
			Entry("delegatee route", delegateeRoute(), parentRouteRef()),
		)

		DescribeTable("should filter out multiple negative route conditions of the same type from report",
			func(obj client.Object, parentRef *gwv1.ParentReference) {
				rm := reports.NewReportMap()
				reporter := reports.NewReporter(&rm)
				reporter.Route(obj).ParentRef(parentRef).SetCondition(reports.RouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})
				reporter.Route(obj).ParentRef(parentRef).SetCondition(reports.RouteCondition{
					Type:   gwv1.RouteConditionResolvedRefs,
					Status: metav1.ConditionFalse,
					Reason: gwv1.RouteReasonBackendNotFound,
				})

				status := rm.BuildRouteStatus(context.Background(), obj, "gloo-gateway")

				Expect(status).NotTo(BeNil())
				Expect(status.Parents).To(HaveLen(1))
				Expect(status.Parents[0].Conditions).To(HaveLen(2))

				resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
			},
			Entry("regular httproute", httpRoute(), parentRef()),
			Entry("regular tcproute", tcpRoute(), parentRef()),
			Entry("delegatee route", delegateeRoute(), parentRouteRef()),
		)

		DescribeTable("should not modify LastTransitionTime for existing conditions that have not changed",
			func(obj client.Object) {
				rm := reports.NewReportMap()

				reporter := reports.NewReporter(&rm)
				// initialize RouteReporter to mimic translation loop (i.e. report gets initialized for all Routes)
				reporter.Route(obj)

				status := rm.BuildRouteStatus(context.Background(), obj, "gloo-gateway")

				Expect(status).NotTo(BeNil())
				Expect(status.Parents).To(HaveLen(1))
				Expect(status.Parents[0].Conditions).To(HaveLen(2))

				resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				oldTransitionTime := resolvedRefs.LastTransitionTime

				// Type assert the object to update the Status field based on its type
				switch route := obj.(type) {
				case *gwv1.HTTPRoute:
					route.Status.RouteStatus = *status
				case *gwv1a2.TCPRoute:
					route.Status.RouteStatus = *status
				default:
					Fail(fmt.Sprintf("unsupported route type: %T", obj))
				}

				status = rm.BuildRouteStatus(context.Background(), obj, "gloo-gateway")

				Expect(status).NotTo(BeNil())
				Expect(status.Parents).To(HaveLen(1))
				Expect(status.Parents[0].Conditions).To(HaveLen(2))

				resolvedRefs = meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				newTransitionTime := resolvedRefs.LastTransitionTime
				Expect(newTransitionTime).To(Equal(oldTransitionTime))
			},
			Entry("regular httproute", httpRoute()),
			Entry("delegatee route", delegateeRoute()),
			Entry("regular tcproute", tcpRoute()),
		)

		DescribeTable("should correctly handle multiple ParentRefs on a route",
			func(obj client.Object) {
				// Add an additional ParentRef to test multiple parent references handling
				switch route := obj.(type) {
				case *gwv1.HTTPRoute:
					route.Spec.ParentRefs = append(route.Spec.ParentRefs, gwv1.ParentReference{
						Name: "additional-gateway",
					})
				case *gwv1a2.TCPRoute:
					route.Spec.ParentRefs = append(route.Spec.ParentRefs, gwv1.ParentReference{
						Name: "additional-gateway",
					})
				default:
					Fail(fmt.Sprintf("unsupported route type: %T", obj))
				}

				rm := reports.NewReportMap()
				reporter := reports.NewReporter(&rm)

				// Initialize RouteReporter to mimic translation loop
				reporter.Route(obj)

				status := rm.BuildRouteStatus(context.Background(), obj, "gloo-gateway")

				Expect(status).NotTo(BeNil())
				Expect(status.Parents).To(HaveLen(2))

				// Check that each parent has the correct number of conditions
				for _, parent := range status.Parents {
					Expect(parent.Conditions).To(HaveLen(2))
				}
			},
			Entry("regular HTTPRoute", httpRoute()),
			Entry("regular TCPRoute", tcpRoute()),
		)

		DescribeTable("should correctly associate multiple routes with shared and separate listeners",
			func(route1, route2 client.Object, listener1, listener2 gwv1.Listener) {
				gw := gw()
				gw.Spec.Listeners = []gwv1.Listener{listener1, listener2}

				// Assign the first listener to the first route's parent ref
				switch r1 := route1.(type) {
				case *gwv1.HTTPRoute:
					r1.Spec.ParentRefs[0].SectionName = ptr.To(gwv1.SectionName(listener1.Name))
				case *gwv1a2.TCPRoute:
					r1.Spec.ParentRefs[0].SectionName = ptr.To(gwv1.SectionName(listener1.Name))
				}

				// Assign the second listener to the second route's parent ref
				switch r2 := route2.(type) {
				case *gwv1.HTTPRoute:
					r2.Spec.ParentRefs[0].SectionName = ptr.To(gwv1.SectionName(listener2.Name))
				case *gwv1a2.TCPRoute:
					r2.Spec.ParentRefs[0].SectionName = ptr.To(gwv1.SectionName(listener2.Name))
				}

				rm := reports.NewReportMap()
				reporter := reports.NewReporter(&rm)

				// Initialize RouteReporter to mimic translation loop
				reporter.Route(route1)
				reporter.Route(route2)

				status1 := rm.BuildRouteStatus(context.Background(), route1, "gloo-gateway")
				status2 := rm.BuildRouteStatus(context.Background(), route2, "gloo-gateway")

				Expect(status1).NotTo(BeNil())
				Expect(status1.Parents[0].Conditions).To(HaveLen(2))

				Expect(status2).NotTo(BeNil())
				Expect(status2.Parents[0].Conditions).To(HaveLen(2))
			},
			Entry("HTTPRoutes with shared and separate listeners",
				httpRoute(), httpRoute(),
				gwv1.Listener{Name: "foo-http", Protocol: gwv1.HTTPProtocolType},
				gwv1.Listener{Name: "bar-http", Protocol: gwv1.HTTPProtocolType},
			),
			Entry("TCPRoutes with shared and separate listeners",
				tcpRoute(), tcpRoute(),
				gwv1.Listener{Name: "foo-tcp", Protocol: gwv1.TCPProtocolType},
				gwv1.Listener{Name: "bar-tcp", Protocol: gwv1.TCPProtocolType},
			),
		)
	})

	DescribeTable("should handle routes with missing parent references gracefully",
		func(route client.Object) {
			// Remove ParentRefs from the route.
			switch r := route.(type) {
			case *gwv1.HTTPRoute:
				r.Spec.ParentRefs = nil
			case *gwv1a2.TCPRoute:
				r.Spec.ParentRefs = nil
			}

			rm := reports.NewReportMap()
			reporter := reports.NewReporter(&rm)

			// Initialize RouteReporter to mimic translation loop
			reporter.Route(route)
			status := rm.BuildRouteStatus(context.Background(), route, "gloo-gateway")

			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(BeEmpty())
		},
		Entry("HTTPRoute with missing parent reference", httpRoute()),
		Entry("TCPRoute with missing parent reference", tcpRoute()),
	)
})

func httpRoute() client.Object {
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route",
			Namespace: "default",
		},
	}
	route.Spec.CommonRouteSpec.ParentRefs = append(route.Spec.CommonRouteSpec.ParentRefs, *parentRef())
	return route
}

func tcpRoute() client.Object {
	route := &gwv1a2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route",
			Namespace: "default",
		},
	}
	route.Spec.CommonRouteSpec.ParentRefs = append(route.Spec.CommonRouteSpec.ParentRefs, *parentRef())
	return route
}

func parentRef() *gwv1.ParentReference {
	return &gwv1.ParentReference{
		Name: "parent",
	}
}

func delegateeRoute() client.Object {
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child-route",
			Namespace: "default",
		},
	}
	route.Spec.CommonRouteSpec.ParentRefs = append(route.Spec.CommonRouteSpec.ParentRefs, *parentRouteRef())
	return route
}

func parentRouteRef() *gwv1.ParentReference {
	return &gwv1.ParentReference{
		Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
		Kind:      ptr.To(gwv1.Kind("HTTPRoute")),
		Name:      "parent-route",
		Namespace: ptr.To(gwv1.Namespace("default")),
	}
}

func gw() *gwv1.Gateway {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}
	gw.Spec.Listeners = append(gw.Spec.Listeners, *listener())
	return gw
}

func listener() *gwv1.Listener {
	return &gwv1.Listener{
		Name: "http",
	}
}

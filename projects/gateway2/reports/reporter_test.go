package reports_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/reports"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Reporting Infrastructure", func() {

	BeforeEach(func() {
	})

	Describe("building gateway status", func() {
		It("should build all positive conditions with an empty report", func() {
			gw := gw()
			rm := reports.NewReportMap()
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
			status := rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))

			acceptedCond := meta.FindStatusCondition(status.Listeners[0].Conditions, string(gwv1.ListenerConditionAccepted))
			oldTransitionTime := acceptedCond.LastTransitionTime

			gw.Status = status
			status = rm.BuildGWStatus(context.Background(), *gw)

			Expect(status).NotTo(BeNil())
			Expect(status.Conditions).To(HaveLen(2))
			Expect(status.Listeners).To(HaveLen(1))
			Expect(status.Listeners[0].Conditions).To(HaveLen(4))

			acceptedCond = meta.FindStatusCondition(status.Listeners[0].Conditions, string(gwv1.ListenerConditionAccepted))
			newTransitionTime := acceptedCond.LastTransitionTime
			Expect(newTransitionTime).To(Equal(oldTransitionTime))
		})

		//TODO(Law): add multiple gws/listener tests
		//TODO(Law): add test confirming transitionTime change when status change
	})

	Describe("building route status", func() {
		It("should build all positive route conditions with an empty report", func() {
			route := route()
			rm := reports.NewReportMap()

			status := rm.BuildRouteStatus(context.Background(), route, "gloo-gateway")

			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			Expect(status.Parents[0].Conditions).To(HaveLen(2))
		})

		It("should correctly set negative route conditions from report and not add extra conditions", func() {
			route := route()
			rm := reports.NewReportMap()
			reporter := reports.NewReporter(&rm)
			reporter.Route(&route).ParentRef(parentRef()).SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonBackendNotFound,
			})

			status := rm.BuildRouteStatus(context.Background(), route, "gloo-gateway")

			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			Expect(status.Parents[0].Conditions).To(HaveLen(2))

			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should filter out multiple negative route conditions of the same type from report", func() {
			route := route()
			rm := reports.NewReportMap()
			reporter := reports.NewReporter(&rm)
			reporter.Route(&route).ParentRef(parentRef()).SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonBackendNotFound,
			})
			reporter.Route(&route).ParentRef(parentRef()).SetCondition(reports.HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionFalse,
				Reason: gwv1.RouteReasonBackendNotFound,
			})

			status := rm.BuildRouteStatus(context.Background(), route, "gloo-gateway")

			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			Expect(status.Parents[0].Conditions).To(HaveLen(2))

			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should not modify LastTransitionTime for existing conditions that have not changed", func() {
			route := route()
			rm := reports.NewReportMap()

			status := rm.BuildRouteStatus(context.Background(), route, "gloo-gateway")

			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			Expect(status.Parents[0].Conditions).To(HaveLen(2))

			resolvedRefs := meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			oldTransitionTime := resolvedRefs.LastTransitionTime

			route.Status = status
			status = rm.BuildRouteStatus(context.Background(), route, "gloo-gateway")

			Expect(status).NotTo(BeNil())
			Expect(status.Parents).To(HaveLen(1))
			Expect(status.Parents[0].Conditions).To(HaveLen(2))

			resolvedRefs = meta.FindStatusCondition(status.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			newTransitionTime := resolvedRefs.LastTransitionTime
			Expect(newTransitionTime).To(Equal(oldTransitionTime))
		})
	})
})

func route() gwv1.HTTPRoute {
	route := gwv1.HTTPRoute{
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
		Name: "http",
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

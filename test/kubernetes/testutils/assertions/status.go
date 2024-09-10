package assertions

import (
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Checks GetNamespacedStatuses status for gloo installation namespace
func (p *Provider) EventuallyResourceStatusMatchesWarningReasons(getter helpers.InputResourceGetter, desiredStatusReasons []string, desiredReporter string, timeout ...time.Duration) {
	ginkgo.GinkgoHelper()

	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
	gomega.Eventually(func(g gomega.Gomega) {
		statusWarningsMatcher := matchers.MatchStatusInNamespace(
			p.glooGatewayContext.InstallNamespace,
			gomega.And(matchers.HaveWarningStateWithReasonSubstrings(desiredStatusReasons...), matchers.HaveReportedBy(desiredReporter)),
		)

		status, err := getResourceNamespacedStatus(getter)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
		g.Expect(status).ToNot(gomega.BeNil())
		g.Expect(status).To(gomega.HaveValue(statusWarningsMatcher))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (p *Provider) EventuallyResourceStatusMatchesRejectedReasons(getter helpers.InputResourceGetter, desiredStatusReasons []string, desiredReporter string, timeout ...time.Duration) {
	ginkgo.GinkgoHelper()

	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
	gomega.Eventually(func(g gomega.Gomega) {
		statusRejectionsMatcher := matchers.MatchStatusInNamespace(
			p.glooGatewayContext.InstallNamespace,
			gomega.And(matchers.HaveRejectedStateWithReasonSubstrings(desiredStatusReasons...), matchers.HaveReportedBy(desiredReporter)),
		)

		status, err := getResourceNamespacedStatus(getter)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
		g.Expect(status).ToNot(gomega.BeNil())
		g.Expect(status).To(gomega.HaveValue(statusRejectionsMatcher))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (p *Provider) EventuallyResourceStatusMatchesState(
	getter helpers.InputResourceGetter,
	desiredState core.Status_State,
	desiredReporter string,
	timeout ...time.Duration,
) {
	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		statusStateMatcher := matchers.MatchStatusInNamespace(
			p.glooGatewayContext.InstallNamespace,
			gomega.And(matchers.HaveState(desiredState), matchers.HaveReportedBy(desiredReporter)),
		)
		status, err := getResourceNamespacedStatus(getter)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
		g.Expect(status).ToNot(gomega.BeNil())
		g.Expect(status).To(gomega.HaveValue(statusStateMatcher))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (p *Provider) EventuallyResourceStatusMatchesSubResource(
	getter helpers.InputResourceGetter,
	desiredSubresourceName string,
	desiredSubresource matchers.SoloKitSubresourceStatus,
	timeout ...time.Duration,
) {
	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		subResourceStatusMatcher := matchers.HaveSubResourceStatusState(desiredSubresourceName, desiredSubresource)
		status, err := getResourceNamespacedStatus(getter)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
		g.Expect(status).ToNot(gomega.BeNil())
		g.Expect(status).To(gomega.HaveValue(subResourceStatusMatcher))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func getResourceNamespacedStatus(getter helpers.InputResourceGetter) (*core.NamespacedStatuses, error) {
	resource, err := getter()
	if err != nil {
		return &core.NamespacedStatuses{}, errors.Wrapf(err, "failed to get resource")
	}

	namespacedStatuses := resource.GetNamespacedStatuses()

	// In newer versions of Gloo Edge we provide a default "empty" status, which allows us to patch it to perform updates
	// As a result, a nil check isn't enough to determine that that status hasn't been reported
	if namespacedStatuses == nil || namespacedStatuses.GetStatuses() == nil {
		return &core.NamespacedStatuses{}, errors.Wrapf(err, "waiting for %v status to be non-empty", resource.GetMetadata().GetName())
	}

	return namespacedStatuses, nil
}

// AssertHTTPRouteStatusContainsSubstring asserts that at least one of the HTTPRoute's route parent statuses contains
// the given message substring.
func (p *Provider) AssertHTTPRouteStatusContainsSubstring(route *gwv1.HTTPRoute, message string) {
	matcher := matchers.HaveKubeGatewayRouteStatus(&matchers.KubeGatewayRouteStatus{
		Custom: gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Parents": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Conditions": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Message": matchers.ContainSubstrings([]string{message}),
				})),
			})),
		}),
	})
	p.Gomega.Expect(route.Status.RouteStatus).To(gomega.HaveValue(matcher))
}

// AssertHTTPRouteStatusContainsSubstring asserts that at least one of the HTTPRoute's route parent statuses contains
// the given reason substring.
func (p *Provider) AssertHTTPRouteStatusContainsReason(route *gwv1.HTTPRoute, reason string) {
	matcher := matchers.HaveKubeGatewayRouteStatus(&matchers.KubeGatewayRouteStatus{
		Custom: gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Parents": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Conditions": gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Reason": matchers.ContainSubstrings([]string{reason}),
				})),
			})),
		}),
	})
	p.Gomega.Expect(route.Status.RouteStatus).To(gomega.HaveValue(matcher))
}

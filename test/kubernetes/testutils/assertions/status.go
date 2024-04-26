package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"

	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// EventuallyResourceStatusMatchesState checks GetNamespacedStatuses status for gloo installation namespace
func EventuallyResourceStatusMatchesState(installNamespace string, getter helpers.InputResourceGetter, desiredStatusState core.Status_State, desiredReporter string, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
		gomega.Eventually(func(g gomega.Gomega) {
			statusStateMatcher := matchers.MatchStatusInNamespace(
				installNamespace,
				gomega.And(matchers.HaveState(desiredStatusState), matchers.HaveReportedBy(desiredReporter)),
			)

			status, err := getResourceNamespacedStatus(getter)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
			g.Expect(status).ToNot(gomega.BeNil())
			g.Expect(*status).To(statusStateMatcher)
		}, currentTimeout, pollingInterval).Should(gomega.Succeed())
	}
}

// Checks GetNamespacedStatuses status for gloo installation namespace
func EventuallyResourceStatusMatchesWarningReasons(installNamespace string, getter helpers.InputResourceGetter, desiredStatusReasons []string, desiredReporter string, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
		gomega.Eventually(func(g gomega.Gomega) {
			statusWarningsMatcher := matchers.MatchStatusInNamespace(
				installNamespace,
				gomega.And(matchers.HaveWarningStateWithReasonSubstrings(desiredStatusReasons...), matchers.HaveReportedBy(desiredReporter)),
			)

			status, err := getResourceNamespacedStatus(getter)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
			g.Expect(status).ToNot(gomega.BeNil())
			g.Expect(*status).To(statusWarningsMatcher)
		}, currentTimeout, pollingInterval).Should(gomega.Succeed())
	}
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

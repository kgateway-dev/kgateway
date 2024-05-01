package assertions

import (
	"context"
	"time"

	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	errors "github.com/rotisserie/eris"
)

// Checks GetNamespacedStatuses status for gloo installation namespace
func (p *Provider) EventuallyResourceStatusMatchesWarningReasons(installNamespace string, getter helpers.InputResourceGetter, desiredStatusReasons []string, desiredReporter string, timeout ...time.Duration) {
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

func (p *Provider) EventuallyResourceStatusMatchesState(
	_ context.Context,
	getter helpers.InputResourceGetter,
	statusMatcher types.GomegaMatcher,
	timeout ...time.Duration,
) {
	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
	p.Gomega.Eventually(func(g gomega.Gomega) {
		status, err := getResourceNamespacedStatus(getter)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
		nsStatus := status.GetStatuses()[p.glooGatewayContext.InstallNamespace]
		g.Expect(nsStatus).ToNot(gomega.BeNil())
		g.Expect(nsStatus).To(gomega.HaveValue(statusMatcher))
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

package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	errors "github.com/rotisserie/eris"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"
)

// EventuallyResourceStatusMatchesState checks GetNamespacedStatuses status for gloo installation namespace
func EventuallyResourceStatusMatchesState(installNamespace string, getter helpers.InputResourceGetter, desiredStatusState core.Status_State, desiredReporter string, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		statusStateMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"State":      gomega.Equal(desiredStatusState),
			"ReportedBy": gomega.Equal(desiredReporter),
		})

		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
		gomega.Eventually(func(g gomega.Gomega) {
			status, err := getResourceNamespacedStatus(getter)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource namespaced status")
			g.Expect(status.GetStatuses()[installNamespace]).ToNot(gomega.BeNil())
			g.Expect(*status.GetStatuses()[installNamespace]).To(statusStateMatcher)
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

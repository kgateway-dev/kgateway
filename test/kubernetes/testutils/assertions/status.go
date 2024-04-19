package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// Checks GetNamespacedStatuses status for gloo installation namespace
func EventuallyResourceStatusMatchesState(installNamespace string, getter helpers.InputResourceGetter, desiredStatusState core.Status_State, desiredReporter string, timeout ...time.Duration) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		statusStateMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"key": gomega.Equal("k8s-gw-deployer-test"),
			"value": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"state":       gomega.Equal(desiredStatusState),
				"reported_by": gomega.Equal(desiredReporter),
			}),
		})

		currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
		gomega.Eventually(func() (*core.NamespacedStatuses, error) {
			return getResourceNamespacedStatus(getter)
		}, currentTimeout, pollingInterval).Should(statusStateMatcher)
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

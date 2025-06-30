//go:build ignore

package helpers

import (
	"fmt"
	"time"

	"github.com/onsi/gomega/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/statusutils"

	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"
)

const (
	defaultEventuallyTimeout         = 30 * time.Second
	defaultEventuallyPollingInterval = 1 * time.Second
)

type ResourceGetter func() (resources.Resource, error)

func EventuallyResourceExists(getter ResourceGetter, intervals ...interface{}) {
	timeoutInterval, pollingInterval := getTimeoutAndPollingIntervalsOrDefault(intervals...)
	gomega.Eventually(func() (bool, error) {
		_, err := getter()
		if err != nil {
			return false, err
		}
		return true, nil
	}, timeoutInterval, pollingInterval).Should(gomega.BeTrue())
}

type InputResourceGetter func() (resources.InputResource, error)
type InputResourceListGetter func() (resources.InputResourceList, error)

func EventuallyResourceAccepted(getter InputResourceGetter, intervals ...interface{}) {
	EventuallyResourceStatusMatchesState(1, getter, core.Status_Accepted, intervals...)
}

func EventuallyResourceAcceptedWithOffset(offset int, getter InputResourceGetter, intervals ...interface{}) {
	EventuallyResourceStatusMatchesState(offset+1, getter, core.Status_Accepted, intervals...)
}
func EventuallyResourceWarning(getter InputResourceGetter, intervals ...interface{}) {
	EventuallyResourceStatusMatchesState(1, getter, core.Status_Warning, intervals...)
}

func EventuallyResourceRejected(getter InputResourceGetter, intervals ...interface{}) {
	EventuallyResourceStatusMatchesState(1, getter, core.Status_Rejected, intervals...)
}

func EventuallyResourceStatusMatchesState(offset int, getter InputResourceGetter, desiredStatusState core.Status_State, intervals ...interface{}) {
	statusStateMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"State": gomega.Equal(desiredStatusState),
	})

	timeoutInterval, pollingInterval := getTimeoutAndPollingIntervalsOrDefault(intervals...)
	gomega.EventuallyWithOffset(offset+1, func() (core.Status, error) {
		return getResourceStatus(getter)
	}, timeoutInterval, pollingInterval).Should(statusStateMatcher)
}

func EventuallyResourceStatusHasReason(offset int, getter InputResourceGetter, desiredStatusMessage string, intervals ...interface{}) {
	statusMessageMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Reason": gomega.ContainSubstring(desiredStatusMessage),
	})

	timeoutInterval, pollingInterval := getTimeoutAndPollingIntervalsOrDefault(intervals...)
	gomega.EventuallyWithOffset(offset+1, func() (core.Status, error) {
		return getResourceStatus(getter)
	}, timeoutInterval, pollingInterval).Should(statusMessageMatcher)
}

func EventuallyResourceStatusMatches(getter InputResourceGetter, desiredMatcher types.GomegaMatcher, intervals ...interface{}) {
	EventuallyResourceStatusMatchesWithOffset(1, getter, desiredMatcher, intervals...)
}

func EventuallyResourceStatusMatchesWithOffset(offset int, getter InputResourceGetter, desiredMatcher types.GomegaMatcher, intervals ...interface{}) {
	timeoutInterval, pollingInterval := getTimeoutAndPollingIntervalsOrDefault(intervals...)
	gomega.EventuallyWithOffset(offset+1, func() (core.Status, error) {
		return getResourceStatus(getter)
	}, timeoutInterval, pollingInterval).Should(desiredMatcher)
}

func getResourceStatus(getter InputResourceGetter) (core.Status, error) {
	resource, err := getter()
	if err != nil {
		return core.Status{}, fmt.Errorf("failed to get resource: %w", err)
	}

	statusClient := statusutils.GetStatusClientFromEnvOrDefault(defaults.GlooSystem)
	status := statusClient.GetStatus(resource)

	// In newer versions of kgateway we provide a default "empty" status, which allows us to patch it to perform updates
	// As a result, a nil check isn't enough to determine that that status hasn't been reported
	// Note: RateLimitConfig statuses can have an empty reporter
	if status == nil {
		return core.Status{}, fmt.Errorf("waiting for %v status to be non-empty: %w", resource.GetMetadata().GetName(), err)
	}

	return *status, nil
}

func EventuallyResourceDeleted(getter InputResourceGetter, intervals ...interface{}) {
	EventuallyResourceDeletedWithOffset(1, getter, intervals...)
}

func EventuallyResourceDeletedWithOffset(offset int, getter InputResourceGetter, intervals ...interface{}) {
	timeoutInterval, pollingInterval := getTimeoutAndPollingIntervalsOrDefault(intervals...)
	gomega.EventuallyWithOffset(offset+1, func() (bool, error) {
		_, err := getter()
		if err != nil && skerrors.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}, timeoutInterval, pollingInterval).Should(gomega.BeTrue())
}

var getTimeoutAndPollingIntervalsOrDefault = GetEventuallyTimingsTransform(defaultEventuallyTimeout, defaultEventuallyPollingInterval)

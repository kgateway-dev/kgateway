package helpers

import (
	"time"

	"github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"
)

const (
	defaultEventuallyTimeout         = 30 * time.Second
	defaultEventuallyPollingInterval = 1 * time.Second
)

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

	statusClient := statusutils.GetStatusClientFromEnvOrDefault(defaults.GlooSystem)

	timeoutInterval, pollingInterval := getTimeoutAndPollingIntervalsOrDefault(intervals...)
	gomega.EventuallyWithOffset(offset+1, func() (core.Status, error) {
		resource, err := getter()
		if err != nil {
			return core.Status{}, errors.Wrapf(err, "failed to get resource")
		}

		status := statusClient.GetStatus(resource)

		// In newer versions of Gloo Edge we provide a default "empty" status, which allows us to patch it to perform updates
		// As a result, a nil check isn't enough to determine that that status hasn't been reported
		if status == nil || status.GetReportedBy() == "" {
			return core.Status{}, errors.Wrapf(err, "waiting for %v status to be non-empty", resource.GetMetadata().GetName())
		}

		return *status, nil
	}, timeoutInterval, pollingInterval).Should(statusStateMatcher)
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

func getTimeoutAndPollingIntervalsOrDefault(intervals ...interface{}) (interface{}, interface{}) {
	var timeoutInterval, pollingInterval interface{}

	timeoutInterval = defaultEventuallyTimeout
	pollingInterval = defaultEventuallyPollingInterval

	if len(intervals) > 0 {
		timeoutInterval = intervals[0]
	}
	if len(intervals) > 1 {
		pollingInterval = intervals[1]
	}

	return timeoutInterval, pollingInterval
}

package helpers

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skerrors "github.com/solo-io/solo-kit/pkg/errors"
)

// PatchResource mutates an existing persisted resource, retrying if a resourceVersionError is encountered
// The mutator method must return the full object that will be persisted, any side effects from the mutator will be ignored
func PatchResource(ctx context.Context, resourceRef *core.ResourceRef, mutator func(resource resources.Resource) resources.Resource, client clients.ResourceClient) error {
	return PatchResourceWithOffset(1, ctx, resourceRef, mutator, client)
}

// PatchResourceWithOffset mutates an existing persisted resource, retrying if a resourceVersionError is encountered
// The mutator method must return the full object that will be persisted, any side effects from the mutator will be ignored
func PatchResourceWithOffset(offset int, ctx context.Context, resourceRef *core.ResourceRef, mutator func(resource resources.Resource) resources.Resource, client clients.ResourceClient) error {
	// There is a potential bug in our resource writing implementation that leads to test flakes
	// https://github.com/solo-io/gloo/issues/7044
	// This is a temporary solution to ensure that tests do not flake

	var patchErr error

	EventuallyWithOffset(offset+1, func(g Gomega) {
		resource, err := client.Read(resourceRef.GetNamespace(), resourceRef.GetName(), clients.ReadOpts{Ctx: ctx})

		g.Expect(err).NotTo(HaveOccurred())
		resourceVersion := resource.GetMetadata().GetResourceVersion()

		mutatedResource := mutator(resource)
		mutatedResource.GetMetadata().ResourceVersion = resourceVersion

		_, patchErr = client.Write(mutatedResource, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
		g.Expect(skerrors.IsResourceVersion(patchErr)).To(BeFalse())
	}, time.Second*5, time.Second).ShouldNot(HaveOccurred())

	return patchErr
}

// PercentileIndex returns the index of percentile pct for a slice of length len
// The Nearest Rank Method is used to determine percentiles (https://en.wikipedia.org/wiki/Percentile#The_nearest-rank_method)
// Valid inputs for pct are 0 < n <= 100, any other input will cause panic
func PercentileIndex(length, pct int) int {
	if pct <= 0 || pct > 100 {
		panic(fmt.Sprintf("percentile must be > 0 and <= 100, given %d", pct))
	}

	return int(math.Ceil(float64(length)*(float64(pct)/float64(100)))) - 1
}

// GetDefaultEventuallyTimeoutsTransform returns timeout and polling interval values to use with a gomega eventually call
// The `defaults` parameter can be used to override the default Gomega values.
// The first value in the `defaults` slice will be used as the timeout, and the second value will be used as the polling interval (if present)
//
// Example usage:
// getTimeouts := GetEventuallyTimingsTransform(5*time.Second, 100*time.Millisecond)
// timeout, pollingInterval := getTimeouts() // returns 5*time.Second, 100*time.Millisecond
// timeout, pollingInterval := getTimeouts(10*time.Second) // returns 10*time.Second, 100*time.Millisecond
// timeout, pollingInterval := getTimeouts(10*time.Second, 200*time.Millisecond) // returns 10*time.Second, 200*time.Millisecond
// See tests for more examples
func GetEventuallyTimingsTransform(defaults ...interface{}) func(intervals ...interface{}) (interface{}, interface{}) {
	return GetDefaultTimingsTransform(gomega.DefaultEventuallyTimeout, gomega.DefaultEventuallyPollingInterval, defaults...)
}

// GetConsistentlyTimingsTransform returns timeout and polling interval values to use with a gomega consistently call
// The `defaults` parameter can be used to override the default Gomega values.
// The first value in the `defaults` slice will be used as the timeout, and the second value will be used as the polling interval (if present)
//
// Example usage:
// getTimeouts := GetConsistentlyTimingsTransform(5*time.Second, 100*time.Millisecond)
// timeout, pollingInterval := getTimeouts() // returns 5*time.Second, 100*time.Millisecond
// timeout, pollingInterval := getTimeouts(10*time.Second) // returns 10*time.Second, 100*time.Millisecond
// timeout, pollingInterval := getTimeouts(10*time.Second, 200*time.Millisecond) // returns 10*time.Second, 200*time.Millisecond
// See tests for more examples
func GetConsistentlyTimingsTransform(defaults ...interface{}) func(intervals ...interface{}) (interface{}, interface{}) {
	return GetDefaultTimingsTransform(gomega.DefaultConsistentlyDuration, gomega.DefaultConsistentlyPollingInterval, defaults...)
}

// GetDefaultTimingsTransform is used to return the timeout and polling interval values to use with a gomega eventually or consistently call
// It can also be called directly with just 2 arguments if both timeout and polling interval are known and there is no need to default to Gomega values
func GetDefaultTimingsTransform(timeout, polling interface{}, defaults ...interface{}) func(intervals ...interface{}) (interface{}, interface{}) {
	var defaultTimeoutInterval, defaultPollingInterval interface{}
	defaultTimeoutInterval = timeout
	defaultPollingInterval = polling

	// The curl helper doesn't let you set the intervals to 0, so we need to check for that
	if len(defaults) > 0 && defaults[0] != 0 {
		defaultTimeoutInterval = defaults[0]
	}
	if len(defaults) > 1 && defaults[1] != 0 {
		defaultPollingInterval = defaults[1]
	}

	// This function is a closure that will return the timeout and polling intervals
	return func(intervals ...interface{}) (interface{}, interface{}) {
		var timeoutInterval, pollingInterval interface{}
		timeoutInterval = defaultTimeoutInterval
		pollingInterval = defaultPollingInterval

		if len(intervals) > 0 && intervals[0] != 0 {
			durationInterval, err := asDuration(intervals[0])
			Expect(err).NotTo(HaveOccurred())
			if durationInterval != 0 {
				timeoutInterval = durationInterval
			}
		}
		if len(intervals) > 1 && intervals[1] != 0 {
			durationInterval, err := asDuration(intervals[1])
			Expect(err).NotTo(HaveOccurred())
			if durationInterval != 0 {
				pollingInterval = durationInterval
			}
		}

		return timeoutInterval, pollingInterval
	}
}

func asDuration(d interface{}) (time.Duration, error) {
	if duration, ok := d.(time.Duration); ok {
		return duration, nil
	}

	if duration, ok := d.(string); ok {
		parsedDuration, err := time.ParseDuration(duration)
		if err != nil {
			return 0, err
		}
		return parsedDuration, nil
	}

	return 0, fmt.Errorf("could not convert %v to time.Duration", d)
}

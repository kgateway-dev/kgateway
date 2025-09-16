package helpers

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/test/gomega"
)

const (
	DefaultTimeout         = time.Second * 60
	DefaultPollingInterval = time.Second * 2
)

var getTimeoutsAsInterfaces = GetDefaultTimingsTransform(DefaultTimeout, DefaultPollingInterval)

func GetTimeouts(timeout ...time.Duration) (currentTimeout, pollingInterval time.Duration) {
	// Convert the timeouts to interface{}s
	interfaceTimeouts := make([]interface{}, len(timeout))
	for i, t := range timeout {
		interfaceTimeouts[i] = t
	}

	timeoutAny, pollingIntervalAny := getTimeoutsAsInterfaces(interfaceTimeouts...)
	currentTimeout = timeoutAny.(time.Duration)
	pollingInterval = pollingIntervalAny.(time.Duration)
	return currentTimeout, pollingInterval
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

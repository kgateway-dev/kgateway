package assertions

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"k8s.io/utils/pointer"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/go-utils/stats"
)

// Gloo Edge exports statistics to provide details about how the system is behaving
// Most stats utilities are defined in: https://github.com/solo-io/go-utils/tree/main/stats
// This file contains a set of assertions that can be performed by tests to ensure that recorded stats
// match what we would expect

const (
	// timeToSyncStats represents the interval at which metrics reporting occurs
	timeToSyncStats = time.Second * 5
	// SafeTimeToSyncStats represents a safe estimate for metrics reporting interval. We provide a buffer beyond
	// the time that metrics reporting occurs, to account for latencies. Tests should use the SafeTimeToSyncStats
	// to ensure that we are polling the stats endpoints infrequently enough that they will have updated each time
	SafeTimeToSyncStats = timeToSyncStats + time.Second*3
)

// StatsPortFwd represents the set of configuration required to generated port-forward to access statistics locally
type StatsPortFwd struct {
	ResourceName      string
	ResourceNamespace string
	LocalPort         int
	TargetPort        int
}

// DefaultStatsPortFwd is a commonly used port-forward configuration, since Gloo Deployment stats are the most valuable
// This is used in Gloo Enterprise
var DefaultStatsPortFwd = StatsPortFwd{
	ResourceName:      "deployment/gloo",
	ResourceNamespace: defaults.GlooSystem,
	LocalPort:         stats.DefaultPort,
	TargetPort:        stats.DefaultPort,
}

// EventuallyStatisticsMatchAssertions first opens a fort-forward and then performs
// a series of Asynchronous assertions. The fort-forward is cleaned up with the function returns
func EventuallyStatisticsMatchAssertions(statsPortFwd StatsPortFwd, assertions ...types.AsyncAssertion) {
	EventuallyWithOffsetStatisticsMatchAssertions(1, statsPortFwd, assertions...)
}

// EventuallyWithOffsetStatisticsMatchAssertions first opens a fort-forward and then performs
// a series of Asynchronous assertions. The fort-forward is cleaned up with the function returns
func EventuallyWithOffsetStatisticsMatchAssertions(offset int, statsPortFwd StatsPortFwd, assertions ...types.AsyncAssertion) {
	portForward, err := cliutil.PortForward(
		statsPortFwd.ResourceNamespace,
		statsPortFwd.ResourceName,
		fmt.Sprintf("%d", statsPortFwd.LocalPort),
		fmt.Sprintf("%d", statsPortFwd.TargetPort),
		false)
	ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())

	defer func() {
		if portForward.Process != nil {
			_ = portForward.Process.Kill()
			_ = portForward.Process.Release()
		}
	}()

	By("Ensure port-forward is open before performing assertions")
	statsRequest, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/", statsPortFwd.LocalPort), nil)
	ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(offset+1, func(g Gomega) {
		g.Expect(http.DefaultClient.Do(statsRequest)).To(matchers.HaveHttpResponse(&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       Not(BeEmpty()),
		}))
	}).Should(Succeed())

	By("Perform the assertions while the port forward is open")
	for _, assertion := range assertions {
		assertion.WithOffset(offset + 1).ShouldNot(HaveOccurred())
	}
}

// IntStatisticReachesConsistentValueAssertion returns an assertion that a prometheus stats has reached a consistent value
// It optionally returns the value of that statistic as well
// Arguments:
//
//	prometheusStat (string) - The name of the statistic we will be evaluating
//	inARow (int) - We periodically poll the statistic value from a metrics endpoint. InARow represents
//				   the number of consecutive times the statistic must be the same for it to be considered "consistent"
//				   For example, if InARow=4, we must poll the endpoint 4 times consecutively and return the same value
func IntStatisticReachesConsistentValueAssertion(prometheusStat string, inARow int) (types.AsyncAssertion, *int) {
	statRegex, err := regexp.Compile(fmt.Sprintf("%s ([\\d]+)", prometheusStat))
	Expect(err).NotTo(HaveOccurred())

	statTransform := transforms.IntRegexTransform(statRegex)

	// Assumes that the metrics are exposed via the default port
	metricsRequest, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d/metrics", stats.DefaultPort), nil)
	Expect(err).NotTo(HaveOccurred())

	var (
		currentlyInARow   = 0
		previousStatValue = 0
		currentStatValue  = pointer.Int(0)
	)

	return Eventually(func(g Gomega) {
		g.Expect(http.DefaultClient.Do(metricsRequest)).To(matchers.HaveHttpResponse(&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: WithTransform(func(body []byte) error {
				statValue, transformErr := statTransform(body)
				*currentStatValue = statValue
				return transformErr
			}, Not(HaveOccurred())),
		}))

		if *currentStatValue == 0 || *currentStatValue != previousStatValue {
			currentlyInARow = 0
		} else {
			currentlyInARow += 1
		}
		previousStatValue = *currentStatValue
		g.Expect(currentlyInARow).To(Equal(inARow))
	}, "2m", SafeTimeToSyncStats), currentStatValue
}

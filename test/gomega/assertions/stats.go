package assertions

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/go-utils/stats"
)

const (
	TimeToSyncStats     = time.Second * 5 // Metrics reporting occurs at 5s intervals
	SafeTimeToSyncStats = TimeToSyncStats + time.Second*2
)

type StatsPortFwd struct {
	ResourceName      string
	ResourceNamespace string
	LocalPort         int
	TargetPort        int
}

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

	defer func() {
		if portForward.Process != nil {
			portForward.Process.Kill()
			portForward.Process.Release()
		}
	}()
	ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())

	By("Ensure port-forward is open before performing assertions")
	statsRequest, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/", statsPortFwd.LocalPort), nil)
	ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(offset+1, func(g Gomega) {
		g.Expect(http.DefaultClient.Do(statsRequest)).To(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       Succeed(),
		}))
	}).Should(Succeed())

	// Perform the assertions while the port forward is open
	for _, assertion := range assertions {
		assertion.WithOffset(offset + 1).ShouldNot(HaveOccurred())
	}
}

// EventuallyIntStatisticReachesConsistentValue returns an assertion that a prometheus stats has reached a consistent value
// It optionally returns the value of that statistic as well
func EventuallyIntStatisticReachesConsistentValue(offset int, prometheusStat string, inARow int) (types.AsyncAssertion, int) {
	statRegex, err := regexp.Compile(fmt.Sprintf("%s ([\\d]+)", prometheusStat))
	ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())

	statTransform := transforms.IntRegexTransform(statRegex)

	// Assumes that the metrics are exposed via the default port
	metricsRequest, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d/metrics", stats.DefaultPort), nil)
	ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())

	var (
		currentlyInARow   = 0
		previousStatValue = 0
		currentStatValue  = 0
	)

	return EventuallyWithOffset(offset+1, func(g Gomega) {
		g.Expect(http.DefaultClient.Do(metricsRequest)).To(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: WithTransform(func(body []byte) error {
				statValue, transformErr := statTransform(body)
				currentStatValue = statValue
				return transformErr
			}, Not(HaveOccurred())),
		}))

		if currentStatValue == 0 || currentStatValue != previousStatValue {
			currentlyInARow = 0
		} else {
			currentlyInARow += 1
		}
		g.Expect(currentlyInARow).Should(Equal(inARow))
	}, "2m", SafeTimeToSyncStats), currentlyInARow
}

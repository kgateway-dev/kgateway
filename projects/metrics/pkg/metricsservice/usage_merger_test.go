package metricsservice

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics merger", func() {
	var (
		usageMerger             *UsageMerger
		currentTime             = time.Date(2019, 4, 20, 16, 20, 0, 0, time.UTC)
		testCurrentTimeProvider = func() time.Time {
			return currentTime
		}
		envoyInstanceId   = "gateway-proxy-v2-84585498d7-lfw6g.gloo-system"
		uptime            = time.Hour
		firstRecordedTime = currentTime.Add(time.Duration(-1) * uptime)
		exampleMetrics    = &EnvoyMetrics{
			HttpRequests:   100,
			TcpConnections: 0,
			Uptime:         uptime,
		}
		existingUsage = &GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics:    exampleMetrics,
					LastRecordedAt:  currentTime.Add(time.Duration(-5) * time.Second),
					FirstRecordedAt: firstRecordedTime,
					Active:          true,
				},
			},
		}
	)

	BeforeEach(func() {
		usageMerger = NewUsageMerger(testCurrentTimeProvider)
	})

	It("works for newly-recorded metrics", func() {
		mergedUsage := usageMerger.MergeUsage(envoyInstanceId, nil, exampleMetrics)

		expected := &GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics:    exampleMetrics,
					LastRecordedAt:  currentTime,
					FirstRecordedAt: currentTime,
					Active:          true,
				},
			},
		}
		Expect(mergedUsage).To(Equal(expected), "Newly-created metrics should be recorded as-is")
	})

	It("works when envoy has not restarted", func() {
		newMetrics := &EnvoyMetrics{
			HttpRequests:   exampleMetrics.HttpRequests + 10,
			TcpConnections: exampleMetrics.TcpConnections + 10,
			Uptime:         exampleMetrics.Uptime + time.Second*5,
		}

		mergedUsage := usageMerger.MergeUsage(envoyInstanceId, existingUsage, newMetrics)

		expected := &GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics:    newMetrics,
					LastRecordedAt:  currentTime,
					FirstRecordedAt: firstRecordedTime,
					Active:          true,
				},
			},
		}

		Expect(mergedUsage).To(Equal(expected), "Metrics should be recorded as-is if envoy has not restarted")
	})

	It("works when envoy has restarted", func() {
		newMetrics := &EnvoyMetrics{
			HttpRequests:   10,
			TcpConnections: 0,
			Uptime:         uptime / 10,
		}

		mergedUsage := usageMerger.MergeUsage(envoyInstanceId, existingUsage, newMetrics)

		expected := &GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics: &EnvoyMetrics{
						HttpRequests:   exampleMetrics.HttpRequests + newMetrics.HttpRequests,
						TcpConnections: exampleMetrics.TcpConnections + newMetrics.TcpConnections,
						Uptime:         newMetrics.Uptime,
					},
					LastRecordedAt:  currentTime,
					FirstRecordedAt: firstRecordedTime,
					Active:          true,
				},
			},
		}

		Expect(mergedUsage).To(Equal(expected), "The metrics should be merged when envoy has restarted")
	})

	It("knows how to mark envoys as inactive", func() {
		laterTime := currentTime.Add(envoyExpiryDuration * time.Duration(2))
		testMerger := NewUsageMerger(func() time.Time {
			return laterTime
		})
		differentEnvoyId := envoyInstanceId + "-different-string"

		newMetrics := &EnvoyMetrics{
			HttpRequests:   3,
			TcpConnections: 3,
			Uptime:         time.Millisecond * 500,
		}

		mergedUsage := testMerger.MergeUsage(differentEnvoyId, existingUsage, newMetrics)

		expected := &GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics:    exampleMetrics,
					LastRecordedAt:  currentTime.Add(time.Duration(-5) * time.Second),
					FirstRecordedAt: firstRecordedTime,
					Active:          false,
				},
				differentEnvoyId: {
					EnvoyMetrics:    newMetrics,
					LastRecordedAt:  laterTime,
					FirstRecordedAt: laterTime,
					Active:          true,
				},
			},
		}

		Expect(mergedUsage).To(Equal(expected), "The old envoy instance should be marked as inactive")
	})
})

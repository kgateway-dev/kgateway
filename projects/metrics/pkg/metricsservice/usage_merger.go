package metricsservice

import "time"

type CurrentTimeProvider func() time.Time

type UsageMerger struct {
	currentTimeProvider CurrentTimeProvider
}

// provide a way to get the current time
// used to make unit tests easier to write and more deterministic
func NewUsageMerger(currentTimeProvider CurrentTimeProvider) *UsageMerger {
	return &UsageMerger{currentTimeProvider: currentTimeProvider}
}

func (u *UsageMerger) MergeUsage(envoyInstanceId string, oldUsage *GlobalUsage, newMetrics *EnvoyMetrics) *GlobalUsage {
	now := u.currentTimeProvider()
	mergedUsage := &GlobalUsage{}

	if oldUsage == nil {
		mergedUsage = &GlobalUsage{
			EnvoyIdToUsage: map[string]*EnvoyUsage{
				envoyInstanceId: {
					EnvoyMetrics:    newMetrics,
					LastRecordedAt:  now,
					FirstRecordedAt: now,
				},
			},
		}
	} else {
		// make sure the map is the same at first
		mergedUsage.EnvoyIdToUsage = copyUsageMap(oldUsage.EnvoyIdToUsage)

		oldUsage, ok := oldUsage.EnvoyIdToUsage[envoyInstanceId]
		var mergedMetrics *EnvoyMetrics

		// if envoy has restarted since the first time we logged any of its metrics, it will be reporting numbers for
		// requests/connections that are unrelated to what we've already recorded, so we have to add it together with what we've already seen
		if ok && hasEnvoyRestartedSinceFirstLog(now, oldUsage, newMetrics) {
			mergedMetrics = &EnvoyMetrics{
				HttpRequests:   oldUsage.EnvoyMetrics.HttpRequests + newMetrics.HttpRequests,
				TcpConnections: oldUsage.EnvoyMetrics.TcpConnections + newMetrics.TcpConnections,
				Uptime:         newMetrics.Uptime, // reset the uptime to the newer uptime - to ensure that we keep merging the stats in this way
			}
		} else {
			// otherwise, we've seen a continuous stream of metrics, and the metrics being recorded now are
			// actually correct as they are- so just record them as-is
			mergedMetrics = newMetrics
		}

		firstRecordedTime := now
		if ok {
			firstRecordedTime = oldUsage.FirstRecordedAt
		}

		mergedUsage.EnvoyIdToUsage[envoyInstanceId] = &EnvoyUsage{
			EnvoyMetrics:    mergedMetrics,
			LastRecordedAt:  now,
			FirstRecordedAt: firstRecordedTime,
		}
	}

	// mark an envoy as inactive after a certain amount of time without a stats ping
	for _, v := range mergedUsage.EnvoyIdToUsage {
		v.Active = now.Sub(v.LastRecordedAt) <= envoyExpiryDuration
	}

	return mergedUsage
}

func hasEnvoyRestartedSinceFirstLog(now time.Time, oldUsage *EnvoyUsage, newMetrics *EnvoyMetrics) bool {
	// if envoy has not restarted, then its uptime should be roughly:
	// (the current time) minus (the time we first received metrics from envoy)
	expectedUptime := now.Sub(oldUsage.FirstRecordedAt)
	actualUptime := newMetrics.Uptime

	uptimeDiff := expectedUptime - actualUptime

	// envoy has restarted if the difference between the expected uptime and the actual uptime
	// is positive - within a small epsilon to account for things like a slow startup
	return uptimeDiff >= uptimeDiffThreshold
}

func copyUsageMap(usages map[string]*EnvoyUsage) map[string]*EnvoyUsage {
	newMap := map[string]*EnvoyUsage{}
	for k, v := range usages {
		copiedUsage := *v
		newMap[k] = &copiedUsage
	}

	return newMap
}

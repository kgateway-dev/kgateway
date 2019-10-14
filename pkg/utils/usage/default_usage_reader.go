package usage

import (
	"fmt"
	"time"

	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice"
	"github.com/solo-io/reporting-client/pkg/client"
)

const (
	ReportingServiceUrl = "reporting.corp.solo.io:443"
	ReportingDuration   = time.Hour * 24

	numEnvoys        = "numActiveEnvoys"
	totalRequests    = "totalRequests"
	totalConnections = "totalConnections"
)

type DefaultUsageReader struct {
	MetricsStorage metricsservice.StorageClient
}

var _ client.UsagePayloadReader = &DefaultUsageReader{}

func (d *DefaultUsageReader) GetPayload() (map[string]string, error) {
	usage, err := d.MetricsStorage.GetUsage()
	if err != nil {
		return nil, err
	}

	payload := map[string]string{}

	if usage == nil || usage.EnvoyIdToUsage == nil {
		return payload, nil
	}

	envoys := 0
	requestsCount := uint64(0)
	connectionsCount := uint64(0)

	for _, envoyUsage := range usage.EnvoyIdToUsage {
		if envoyUsage.Active {
			envoys++
			requestsCount += envoyUsage.EnvoyMetrics.HttpRequests
			connectionsCount += envoyUsage.EnvoyMetrics.TcpConnections
		}
	}

	payload[numEnvoys] = fmt.Sprintf("%d", envoys)

	if requestsCount > 0 {
		payload[totalRequests] = fmt.Sprintf("%d", requestsCount)
	}
	if connectionsCount > 0 {
		payload[totalConnections] = fmt.Sprintf("%d", totalConnections)
	}

	return payload, nil
}

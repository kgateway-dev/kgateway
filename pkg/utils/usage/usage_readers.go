package usage

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice"
	"github.com/solo-io/reporting-client/pkg/client"
)

const (
	// this is the url for a grpc service
	// note that grpc name resolution is a little different than for a normal HTTP/1.1 service
	// https://github.com/grpc/grpc/blob/master/doc/naming.md
	ReportingServiceUrl = "reporting.corp.solo.io:443"

	// report once per period
	ReportingPeriod = time.Hour * 24

	numEnvoys        = "numActiveEnvoys"
	totalRequests    = "totalRequests"
	totalConnections = "totalConnections"
	args             = "args"
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
	requestsCount := float64(0)
	connectionsCount := float64(0)

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
		payload[totalConnections] = fmt.Sprintf("%d", connectionsCount)
	}

	return payload, nil
}

type CliUsageReader struct {
}

var _ client.UsagePayloadReader = &CliUsageReader{}

// when reporting usage, also include the args that glooctl was invoked with
func (c *CliUsageReader) GetPayload() (map[string]string, error) {
	argsMap := map[string]string{}

	if len(os.Args) > 1 {
		// don't report the binary name, which will be the first arg
		// it may contain paths on the user's computer
		argsMap[args] = strings.Join(os.Args[1:], "|")
	}

	return argsMap, nil
}

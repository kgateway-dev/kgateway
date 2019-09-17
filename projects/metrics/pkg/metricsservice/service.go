package metricsservice

import (
	"time"

	envoymet "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/solo-io/go-utils/contextutils"
)

// server is used to implement envoyals.AccessLogServiceServer.

type Server struct {
	opts *Options
}

type GlooUsageMetrics map[string]EnvoyUsageMetrics

type EnvoyUsageMetricsEntry struct {
	Requests uint64
	Timestamp time.Time
}

type EnvoyUsageMetrics struct {
	Entries []EnvoyUsageMetricsEntry
}

func (s *Server) StreamMetrics(envoyMetrics envoymet.MetricsService_StreamMetricsServer) error {
	logger := contextutils.LoggerFrom(envoyMetrics.Context())
	met, err := envoyMetrics.Recv()
	if err != nil {
		logger.Debugw("received error from metrics GRPC service")
		return err
	}
	logger.Debugw("successfully received metrics message from envoy")

}

type Options struct {
}

func NewServer(opts Options) *Server {
	return &Server{opts: &opts}
}

package metricsservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	envoymet "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
	"go.uber.org/zap"
	prometheus "istio.io/gogo-genproto/prometheus"
)

const (
	ReadConfigStatPrefix = "read_config"
	PrometheusStatPrefix = "prometheus"

	ServerUptime = "server.uptime"

	TcpStatPrefix      = "tcp"
	HttpStatPrefix     = "http"
	ListenerStatPrefix = "listener"
)

// server is used to implement envoymet.MetricsServiceServer.
type Server struct {
	opts *Options
}

var _ envoymet.MetricsServiceServer = new(Server)

type GlooUsageMetrics map[string]EnvoyUsageMetrics

func AddMetric(id *envoymet.StreamMetricsMessage_Identifier) {

}

func GetNodeCount() int {
	return 0
}

type EnvoyUsageMetricsEntry struct {
	Requests  uint64
	Timestamp time.Time
}

type EnvoyUsageMetrics struct {
	Entries []EnvoyUsageMetricsEntry
}

func (s *Server) StreamMetrics(envoyMetrics envoymet.MetricsService_StreamMetricsServer) error {
	logger := contextutils.LoggerFrom(s.opts.Ctx)
	met, err := envoyMetrics.Recv()
	if err != nil {
		logger.Debugw("received error from metrics GRPC service")
		return err
	}
	logger.Infow("successfully received metrics message from envoy",
		zap.String("cluster.cluster", met.Identifier.Node.Cluster),
		zap.String("cluster.id", met.Identifier.Node.Id),
		zap.Any("cluster.metadata", met.Identifier.Node.Metadata),
		zap.Int("number of metrics", len(met.EnvoyMetrics)),
	)

	tempMetricsMap := make(map[string])

	for _, v := range met.EnvoyMetrics {
		switch {
		case strings.HasPrefix(v.GetName(), ListenerStatPrefix) && strings.HasSuffix(v.GetName(), "downstream_rq_completed"):
			logger.Infof("downstream_rq_completed")
		case strings.HasPrefix(v.GetName(), TcpStatPrefix) && strings.HasSuffix(v.GetName(), "downstream_cx_total"):
		}
	}
	return nil
}

func getUptime(metrics []*prometheus.MetricFamily) (time.Duration, error) {
	for _, metricFamily := range metrics {
		if metricFamily.GetName() == ServerUptime {
			for _, metric := range metricFamily.GetMetric() {
				gauge := metric.GetGauge()
				if gauge == nil {
					continue
				}
				return time.ParseDuration(fmt.Sprintf("%fs", gauge.GetValue()))
			}
		}
	}
	return 0, errors.New("could not get server uptime")
}

type Options struct {
	Ctx context.Context
}

func NewServer(opts Options) *Server {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	return &Server{opts: &opts}
}

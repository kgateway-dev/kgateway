package metricsservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	envoymet "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/solo-io/go-utils/contextutils"
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
	opts    *Options
	storage Storage
}

var _ envoymet.MetricsServiceServer = new(Server)

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

	newUsage, err := s.buildNewUsage(s.opts.Ctx, met)
	if err != nil {
		logger.Errorf("Error while building new usage: %s", err.Error())
		return err
	}

	err = s.storage.ReceiveMetrics(s.opts.Ctx, met.Identifier.Node.Id, newUsage)
	if err != nil {
		logger.Errorf("Error while storing new usage: %s", err.Error())
		return err
	}
	return nil
}

func (s *Server) buildNewUsage(ctx context.Context, metricsMessage *envoymet.StreamMetricsMessage) (*EnvoyMetrics, error) {
	newMetricsEntry := &EnvoyMetrics{}

	for _, v := range metricsMessage.EnvoyMetrics {
		name := v.GetName()
		switch {
		// ignore cluster-specific stats, like accesses to the admin port cluster
		case strings.HasPrefix(name, "cluster") || strings.HasPrefix(name, HttpStatPrefix+".admin"):
			continue
		// ignore the static listeners that we explicitly create
		case strings.HasPrefix(name, HttpStatPrefix+".prometheus") || strings.HasPrefix(name, HttpStatPrefix+".read_config"):
			continue
		case strings.HasPrefix(name, HttpStatPrefix) && strings.HasSuffix(name, "downstream_rq_total"):
			newMetricsEntry.HttpRequests += sumMetricCounter(v.Metric)
		case strings.HasPrefix(name, TcpStatPrefix) && strings.HasSuffix(name, "downstream_cx_total"):
			newMetricsEntry.TcpConnections += sumMetricCounter(v.Metric)
		case v.GetName() == ServerUptime:
			uptime := sumMetricGauge(v.Metric)
			uptimeDuration, err := time.ParseDuration(fmt.Sprintf("%ds", uptime))
			if err != nil {
				return nil, err
			}
			newMetricsEntry.Uptime = uptimeDuration
		}
	}

	return newMetricsEntry, nil
}

func sumMetricCounter(metrics []*prometheus.Metric) uint64 {
	var sum uint64
	for _, m := range metrics {
		sum += uint64(m.Counter.Value)
	}

	return sum
}

func sumMetricGauge(metrics []*prometheus.Metric) uint64 {
	var sum uint64
	for _, m := range metrics {
		sum += uint64(m.Gauge.Value)
	}

	return sum
}

type Options struct {
	Ctx context.Context
}

func NewServer(opts Options, storage Storage) *Server {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	return &Server{
		opts:    &opts,
		storage: storage,
	}
}

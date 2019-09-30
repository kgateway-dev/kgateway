package metricsservice

import (
	"context"

	envoymet "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
)

const (
	ReadConfigStatPrefix = "read_config"
	PrometheusStatPrefix = "prometheus"

	ServerUptime = "server.uptime"

	TcpStatPrefix      = "tcp"
	HttpStatPrefix     = "http"
	ListenerStatPrefix = "listener"
)

//go:generate mockgen -destination mocks/mock_metrics_stream.go -package mocks github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2 MetricsService_StreamMetricsServer

// server is used to implement envoymet.MetricsServiceServer.
type Server struct {
	opts           *Options
	metricsHandler MetricsHandler
}

type Options struct {
	Ctx context.Context
}

func NewServer(opts Options, handler MetricsHandler) *Server {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	return &Server{
		opts:           &opts,
		metricsHandler: handler,
	}
}

var _ envoymet.MetricsServiceServer = new(Server)

func (s *Server) StreamMetrics(envoyMetrics envoymet.MetricsService_StreamMetricsServer) error {
	logger := contextutils.LoggerFrom(s.opts.Ctx)
	met, err := envoyMetrics.Recv()
	if err != nil {
		logger.Debugw("received error from metrics GRPC service")
		return err
	}
	logger.Debugw("successfully received metrics message from envoy",
		zap.String("cluster.cluster", met.Identifier.Node.Cluster),
		zap.String("cluster.id", met.Identifier.Node.Id),
		zap.Any("cluster.metadata", met.Identifier.Node.Metadata),
		zap.Int("number of metrics", len(met.EnvoyMetrics)),
	)

	return s.metricsHandler.HandleMetrics(s.opts.Ctx, met)
}

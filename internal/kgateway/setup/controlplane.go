package setup

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"math"
	"net"

	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoylog "github.com/envoyproxy/go-control-plane/pkg/log"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"istio.io/istio/pkg/security"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/krtxds"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/nack"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	xdsSubsystem = "xds"
)

var (
	xdsAuthRequestTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: xdsSubsystem,
			Name:      "auth_rq_total",
			Help:      "Total number of xDS auth requests",
		}, nil)

	xdsAuthSuccessTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: xdsSubsystem,
			Name:      "auth_rq_success_total",
			Help:      "Total number of successful xDS auth requests",
		}, nil)

	xdsAuthFailureTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: xdsSubsystem,
			Name:      "auth_rq_failure_total",
			Help:      "Total number of failed xDS auth requests",
		}, nil)
)

// slogAdapterForEnvoy adapts *slog.Logger to envoylog.Logger interface
type slogAdapterForEnvoy struct {
	logger *slog.Logger
}

// Ensure it implements the interface
var _ envoylog.Logger = (*slogAdapterForEnvoy)(nil)

func (s *slogAdapterForEnvoy) Debugf(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelDebug) {
		s.logger.Debug(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Infof(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelInfo) {
		s.logger.Info(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Warnf(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelWarn) {
		s.logger.Warn(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Errorf(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelError) {
		s.logger.Error(fmt.Sprintf(format, args...)) //nolint:sloglint // ignore formatting
	}
}

func NewControlPlane(
	ctx context.Context,
	lis net.Listener,
	callbacks xdsserver.Callbacks,
	authenticators []security.Authenticator,
	xdsAuth bool,
	certWatcher *certwatcher.CertWatcher,
) envoycache.SnapshotCache {
	baseLogger := slog.Default().With("component", "envoy-controlplane")
	envoyLoggerAdapter := &slogAdapterForEnvoy{logger: baseLogger}

	// Create separate gRPC servers for each listener
	serverOpts := getGRPCServerOpts(authenticators, xdsAuth, certWatcher, baseLogger)
	kgwGRPCServer := grpc.NewServer(serverOpts...)

	snapshotCache := envoycache.NewSnapshotCache(true, xds.NewNodeRoleHasher(), envoyLoggerAdapter)

	xdsServer := xdsserver.NewServer(ctx, snapshotCache, callbacks)

	// Register reflection and services on both servers
	reflection.Register(kgwGRPCServer)

	// Register xDS services on first server
	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(kgwGRPCServer, xdsServer)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(kgwGRPCServer, xdsServer)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(kgwGRPCServer, xdsServer)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(kgwGRPCServer, xdsServer)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(kgwGRPCServer, xdsServer)

	// Start both servers on their respective listeners
	go kgwGRPCServer.Serve(lis)

	// Handle graceful shutdown for both servers
	go func() {
		<-ctx.Done()
		kgwGRPCServer.GracefulStop()
	}()

	return snapshotCache
}

func NewAgwControlPlane(
	ctx context.Context,
	lis net.Listener,
	authenticators []security.Authenticator,
	xdsAuth bool,
	certWatcher *certwatcher.CertWatcher,
	eventPublisher *nack.NackEventPublisher,
	reg ...krtxds.Registration,
) {
	baseLogger := slog.Default().With("component", "agentgateway-controlplane")

	serverOpts := getGRPCServerOpts(authenticators, xdsAuth, certWatcher, baseLogger)
	grpcServer := grpc.NewServer(serverOpts...)

	ds := krtxds.NewDiscoveryServer(nil, eventPublisher, reg...)
	stop := make(chan struct{})
	context.AfterFunc(ctx, func() {
		close(stop)
	})
	ds.Start(stop)

	reflection.Register(grpcServer)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, ds)

	baseLogger.Info("starting server", "address", lis.Addr().String())
	go grpcServer.Serve(lis)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
}

func getGRPCServerOpts(
	authenticators []security.Authenticator,
	xdsAuth bool,
	certWatcher *certwatcher.CertWatcher,
	logger *slog.Logger,
) []grpc.ServerOption {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_zap.StreamServerInterceptor(zap.NewNop()),
				func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
					slog.Debug("gRPC call", "method", info.FullMethod)
					if xdsAuth {
						xdsAuthRequestTotal.Inc()
						am := authenticationManager{
							Authenticators: authenticators,
						}
						if u := am.authenticate(ss.Context()); u != nil {
							xdsAuthSuccessTotal.Inc()
							return handler(srv, &grpc_middleware.WrappedServerStream{
								ServerStream:   ss,
								WrappedContext: context.WithValue(ss.Context(), xds.PeerCtxKey, u),
							})
						}
						xdsAuthFailureTotal.Inc()
						slog.Error("authentication failed", "reasons", am.authFailMsgs)
						return fmt.Errorf("authentication failed: %v", am.authFailMsgs)
					} else {
						slog.Warn("xDS authentication is disabled")
						return handler(srv, ss)
					}
				},
			)),
	}

	// Add TLS credentials if the certificate watcher was provided. Needed to react to
	// certificate rotations to ensure we're always serving the latest CA certificate.
	if certWatcher != nil {
		creds := credentials.NewTLS(&tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certWatcher.GetCertificate,
		})
		opts = append(opts, grpc.Creds(creds))
		logger.Info("TLS enabled for xDS servers with certificate watcher")
	} else {
		logger.Warn("TLS disabled for xDS servers: connections will be unencrypted")
	}

	return opts
}

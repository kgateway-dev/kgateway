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
	serverconfig "github.com/envoyproxy/go-control-plane/pkg/server/config"
	sotwv3 "github.com/envoyproxy/go-control-plane/pkg/server/sotw/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"istio.io/istio/pkg/security"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	xdsSubsystem = "xds"
)

var (
	controlPlaneLogger = logging.New("envoy-controlplane")

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
		s.logger.Debug(fmt.Sprintf(format, boundLogArgs(args)...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Infof(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelInfo) {
		s.logger.Info(fmt.Sprintf(format, boundLogArgs(args)...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Warnf(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelWarn) {
		s.logger.Warn(fmt.Sprintf(format, boundLogArgs(args)...)) //nolint:sloglint // ignore formatting
	}
}

func (s *slogAdapterForEnvoy) Errorf(format string, args ...any) {
	if s.logger.Enabled(context.Background(), slog.LevelError) {
		s.logger.Error(fmt.Sprintf(format, boundLogArgs(args)...)) //nolint:sloglint // ignore formatting
	}
}

const (
	// maxLoggedNames caps the resource names printed per go-control-plane log
	// line. Its ADS cache warns with every requested name when it holds back a
	// response, which at tens of thousands of routes is ~170 KB per line.
	maxLoggedNames = 10
	// maxLoggedErrLen caps the error text printed per go-control-plane log
	// line; the same warning's error embeds the unrequested names too.
	maxLoggedErrLen = 1024
)

// boundedNames prints at most maxLoggedNames names followed by the total.
type boundedNames []string

func (b boundedNames) String() string {
	return fmt.Sprintf("%v ... (%d names total)", []string(b[:maxLoggedNames]), len(b))
}

// boundLogArgs bounds the resource-name lists and errors that go-control-plane
// passes to its logger, so a single line cannot grow with the snapshot size.
func boundLogArgs(args []any) []any {
	out := make([]any, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case []string:
			if len(v) > maxLoggedNames {
				arg = boundedNames(v)
			}
		case error:
			if msg := v.Error(); len(msg) > maxLoggedErrLen {
				arg = fmt.Sprintf("%s ... (%d bytes truncated)", msg[:maxLoggedErrLen], len(msg)-maxLoggedErrLen)
			}
		}
		out[i] = arg
	}
	return out
}

func NewControlPlane(
	ctx context.Context,
	lis net.Listener,
	callbacks xdsserver.Callbacks,
	authenticators []security.Authenticator,
	xdsAuth bool,
	certWatcher *certwatcher.CertWatcher,
	orderedADS bool,
) envoycache.SnapshotCache {
	envoyLoggerAdapter := &slogAdapterForEnvoy{logger: controlPlaneLogger}
	lnc := newLogNackCallback()
	allCallbacks := chainCallbacks(callbacks, lnc)

	// Create separate gRPC servers for each listener
	// The interceptor uses the per-stream context supplied by gRPC, not the control-plane lifecycle context.
	serverOpts := getGRPCServerOpts(authenticators, xdsAuth, certWatcher) //nolint:contextcheck
	kgwGRPCServer := grpc.NewServer(serverOpts...)

	snapshotCache := envoycache.NewSnapshotCache(true, xds.NewNodeRoleHasher(), envoyLoggerAdapter)

	var xdsOpts []serverconfig.XDSOption
	if orderedADS {
		xdsOpts = append(xdsOpts, sotwv3.WithOrderedADS())
	}
	xdsServer := xdsserver.NewServer(ctx, snapshotCache, allCallbacks, xdsOpts...)

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

func getGRPCServerOpts(
	authenticators []security.Authenticator,
	xdsAuth bool,
	certWatcher *certwatcher.CertWatcher,
) []grpc.ServerOption {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_zap.StreamServerInterceptor(zap.NewNop()),
				func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
					controlPlaneLogger.Debug("gRPC call", "method", info.FullMethod)
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
						controlPlaneLogger.Error("authentication failed", "reasons", am.authFailMsgs)
						return fmt.Errorf("authentication failed: %v", am.authFailMsgs)
					} else {
						controlPlaneLogger.Warn("xDS authentication is disabled")
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
		controlPlaneLogger.Info("TLS enabled for xDS servers with certificate watcher")
	} else {
		controlPlaneLogger.Warn("TLS disabled for xDS servers: connections will be unencrypted")
	}

	return opts
}

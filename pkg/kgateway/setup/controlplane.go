package setup

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"slices"
	"sync"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoylog "github.com/envoyproxy/go-control-plane/pkg/log"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
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

// newSnapshotCache returns the ADS snapshot cache, with EDS, LDS, RDS and SDS
// served per request (perTypeCache).
func newSnapshotCache(hash envoycache.NodeHash, logger envoylog.Logger) envoycache.SnapshotCache {
	c := &perTypeCache{
		SnapshotCache: envoycache.NewSnapshotCache(true, hash, logger),
		byType:        make(map[string]envoycache.SnapshotCache, len(perTypeOrder)),
	}
	for _, typeURL := range perTypeOrder {
		c.byType[typeURL] = envoycache.NewSnapshotCache(false, hash, logger)
	}
	return c
}

// perTypeOrder lists the types perTypeCache serves itself, in publish order.
var perTypeOrder = []string{envoyresource.SecretType, envoyresource.EndpointType, envoyresource.ListenerType, envoyresource.RouteType}

// perTypeCache serves EDS, RDS and SDS watches from a non-ADS copy of each
// snapshot's resources of that type. In ADS mode go-control-plane answers a
// named request only when the snapshot holds nothing outside it, and keeps no
// watch for a new request it declines. Envoy never requests the endpoints,
// routes or secrets of a cluster or listener it rejected, so one rejected
// resource would hold back every update of that type for the proxy, and the
// proxy would stay stale after the resource is removed
// (ads_rejected_resource_test.go). A non-ADS cache answers the requested names.
// Keeping declined watches permits recovery after the blocking resource is
// removed, but cannot deliver updates while it remains. Remove this wrapper
// only when the upstream cache alone passes all six rejected-resource delivery
// scenarios, including updates while a rejected resource remains published.
//
// LDS is served the same way only to keep ADS's order: SetSnapshot publishes
// secrets first, so they are ready when a new listener requests them, then
// CDS, EDS, LDS and RDS. CDS, delta watches and Fetch stay on the embedded ADS
// cache, which stores the whole snapshot for GetSnapshot. Status combines all
// the caches.
type perTypeCache struct {
	envoycache.SnapshotCache
	byType map[string]envoycache.SnapshotCache

	mutationMu sync.Mutex
	mutations  map[string]*nodeCacheMutation
}

type nodeCacheMutation struct {
	mu    sync.Mutex
	users int // includes callers waiting for mu
}

// lockNode serializes installs and clears for one node. Count waiting callers
// too, so the lock can be reclaimed without creating a second lock for that node.
func (c *perTypeCache) lockNode(node string) func() {
	c.mutationMu.Lock()
	if c.mutations == nil {
		c.mutations = make(map[string]*nodeCacheMutation)
	}
	m := c.mutations[node]
	if m == nil {
		m = &nodeCacheMutation{}
		c.mutations[node] = m
	}
	m.users++
	c.mutationMu.Unlock()

	m.mu.Lock()
	return func() {
		m.mu.Unlock()
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		m.users--
		if m.users == 0 {
			delete(c.mutations, node)
		}
	}
}

func (c *perTypeCache) CreateWatch(request *envoycache.Request, sub envoycache.Subscription, value chan envoycache.Response) (func(), error) {
	if typed, ok := c.byType[request.GetTypeUrl()]; ok {
		return typed.CreateWatch(request, sub, value)
	}
	return c.SnapshotCache.CreateWatch(request, sub, value)
}

// SetSnapshot stores the snapshot in every cache even if a response fails.
// Installs and clears for the same node cannot overlap; requests still can.
func (c *perTypeCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	return c.setSnapshot(ctx, node, snapshot, nil)
}

// setSnapshot installs snapshot one cache at a time, in publish order, and
// calls installed, if set, after each install with the type it answered (CDS
// for the ADS cache). The install is not atomic across types: a request can
// arrive between two installs and see the previous version of its type, and
// the next install answers it. Tests use installed to act between installs.
func (c *perTypeCache) setSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot, installed func(typeURL string)) error {
	unlock := c.lockNode(node)
	defer unlock()

	set := func(typeURL string) error {
		only := &envoycache.Snapshot{}
		only.Resources[envoycache.GetResponseType(typeURL)] = envoycache.Resources{
			Version: snapshot.GetVersion(typeURL),
			Items:   snapshot.GetResourcesAndTTL(typeURL),
		}
		err := c.byType[typeURL].SetSnapshot(ctx, node, only)
		if installed != nil {
			installed(typeURL)
		}
		return err
	}
	err := set(envoyresource.SecretType)
	err = errors.Join(err, c.SnapshotCache.SetSnapshot(ctx, node, snapshot))
	if installed != nil {
		installed(envoyresource.ClusterType)
	}
	for _, typeURL := range perTypeOrder[1:] {
		err = errors.Join(err, set(typeURL))
	}
	return err
}

func (c *perTypeCache) ClearSnapshot(node string) {
	unlock := c.lockNode(node)
	defer unlock()

	for _, typeURL := range perTypeOrder {
		c.byType[typeURL].ClearSnapshot(node)
	}
	c.SnapshotCache.ClearSnapshot(node)
}

func (c *perTypeCache) GetStatusInfo(node string) envoycache.StatusInfo {
	var infos combinedStatus
	for _, cache := range c.caches() {
		// GetStatusInfo logs a warning for a node it does not track.
		if slices.Contains(cache.GetStatusKeys(), node) {
			if info := cache.GetStatusInfo(node); info != nil {
				infos = append(infos, info)
			}
		}
	}
	if len(infos) == 0 {
		return nil
	}
	return infos
}

func (c *perTypeCache) GetStatusKeys() []string {
	var keys []string
	for _, cache := range c.caches() {
		for _, key := range cache.GetStatusKeys() {
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

func (c *perTypeCache) caches() []envoycache.SnapshotCache {
	caches := []envoycache.SnapshotCache{c.SnapshotCache}
	for _, typeURL := range perTypeOrder {
		caches = append(caches, c.byType[typeURL])
	}
	return caches
}

// combinedStatus is one node's status across perTypeCache's caches.
type combinedStatus []envoycache.StatusInfo

func (s combinedStatus) GetNode() *envoycorev3.Node { return s[0].GetNode() }

func (s combinedStatus) GetNumWatches() int {
	n := 0
	for _, info := range s {
		n += info.GetNumWatches()
	}
	return n
}

func (s combinedStatus) GetNumDeltaWatches() int {
	n := 0
	for _, info := range s {
		n += info.GetNumDeltaWatches()
	}
	return n
}

func (s combinedStatus) GetLastWatchRequestTime() time.Time {
	var last time.Time
	for _, info := range s {
		if t := info.GetLastWatchRequestTime(); t.After(last) {
			last = t
		}
	}
	return last
}

func (s combinedStatus) GetLastDeltaWatchRequestTime() time.Time {
	var last time.Time
	for _, info := range s {
		if t := info.GetLastDeltaWatchRequestTime(); t.After(last) {
			last = t
		}
	}
	return last
}

func NewControlPlane(
	ctx context.Context,
	lis net.Listener,
	callbacks xdsserver.Callbacks,
	authenticators []security.Authenticator,
	xdsAuth bool,
	certWatcher *certwatcher.CertWatcher,
	orderedADS bool,
	suppressNackResend bool,
	respondOnReconnect bool,
) envoycache.SnapshotCache {
	envoyLoggerAdapter := &slogAdapterForEnvoy{logger: controlPlaneLogger}
	lnc := newLogNackCallback(xdsAuth)
	// The rejection metrics consume the role resolved by unique-client processing.
	// Keep that callback first so failed requests never reach the metric callback.
	allCallbacks := chainCallbacks(callbacks, lnc)

	// Create separate gRPC servers for each listener
	// The interceptor uses the per-stream context supplied by gRPC, not the control-plane lifecycle context.
	serverOpts := getGRPCServerOpts(authenticators, xdsAuth, certWatcher) //nolint:contextcheck
	kgwGRPCServer := grpc.NewServer(serverOpts...)

	hasher := xds.NewNodeRoleHasher()
	var snapshotCache envoycache.SnapshotCache = newSnapshotCache(hasher, envoyLoggerAdapter)
	if suppressNackResend || respondOnReconnect {
		// See watchPolicy: a NACK of the current snapshot version parks the watch
		// instead of re-sending the rejected response, and a reconnecting proxy's
		// first endpoint request is answered regardless of the version it holds.
		policy := newWatchPolicy(snapshotCache, hasher, suppressNackResend, respondOnReconnect)
		allCallbacks = chainCallbacks(allCallbacks, policy.callbacks())
		snapshotCache = policy
	}

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

package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// This file reproduces the STRICT-validation EDS blackout reported against v2.4.1.
// A BackendConfigPolicy is patched to be invalid:
//
//	kubectl patch backendconfigpolicy jwt-test-echo-tls -n jwt-test --type=merge \
//	  -p '{"spec":{"tls":{"parameters":{"cipherSuites":["BOGUS_CIPHER_SUITE_1"]}}}}'
//
// The policy is correctly rejected ("invalid xds configuration: ... Failed to
// initialize cipher suites BOGUS_CIPHER_SUITE_1"), but the controller then logs
//
//	ADS mode: not responding to request .../ClusterLoadAssignment [<other clusters>]:
//	  "kube_jwt-test_echo-server_443" not listed
//
// and endpoint updates stop flowing for every *other* cluster on that proxy.
//
// Mechanism, each link verified against the code in this repo:
//
//  1. backendconfigpolicy's validateXDS attaches the Envoy validation error to the
//     PolicyWrapper (plugin.go), so runPolicies -> TranslateBackendBase records
//     an error plus a blackhole cluster (irtranslator/backend.go).
//  2. snapshotPerClient drops errored clusters from the CDS payload (perclient.go:
//     `if c.Error != nil { ... continue }`), so echo-server's cluster is no longer
//     sent to Envoy. Envoy removes the cluster and unsubscribes from its EDS
//     resource.
//  3. The CLA for that cluster is still published: the endpoint collection follows
//     the backend lifecycle (effective_endpoints.go), not the cluster's error
//     state, and nothing filters CLAs for errored clusters -
//     filterEndpointResourcesForStaticClusters only covers STATIC clusters.
//  4. The snapshot cache runs in ADS mode, where respond() requires the request's
//     resource names to be a superset of the snapshot's resources
//     (go-control-plane cache/v3/simple.go: superset()). A single orphan CLA that
//     Envoy did not ask for makes the entire EDS response be withheld - and
//     respondSOTWWatches deletes the watch anyway, so the proxy is left waiting
//     forever. Endpoints for healthy clusters freeze until Envoy reconnects.
//
// Both tests below fail without the errored-cluster CLA filtering in perclient.go.

const (
	reproHealthyCluster = "kube_default_service1_443"
	reproBrokenCluster  = "kube_jwt-test_echo-server_443"

	// The error STRICT mode surfaces for the patched BackendConfigPolicy.
	reproValidationErr = `invalid xds configuration: error initializing configuration '/dev/fd/0': ` +
		`Failed to initialize cipher suites BOGUS_CIPHER_SUITE_1. The following ciphers were ` +
		`rejected when tried individually: BOGUS_CIPHER_SUITE_1`
)

// TestSnapshotPerClientExcludesErroredClusterCLA pins the invariant ADS mode
// requires: every resource in the snapshot must be one the proxy is able to ask
// for. A cluster dropped from CDS because its BackendConfigPolicy failed STRICT
// validation must not leave its CLA behind in the EDS payload.
func TestSnapshotPerClientExcludesErroredClusterCLA(t *testing.T) {
	g := gomega.NewWithT(t)

	f := newErroredClusterFixture(t)

	// The BackendConfigPolicy on echo-server is patched with a bogus cipher
	// suite: STRICT validation rejects it, so the backend translates to an
	// errored (blackhole) cluster.
	f.breakCluster(reproBrokenCluster)

	snap := f.awaitSnapshot(g, func(s XdsSnapWrapper) bool {
		return len(s.erroredClusters) == 1
	})

	cdsNames := resourceNames(snap.snap.Resources[envoycachetypes.Cluster])
	edsNames := resourceNames(snap.snap.Resources[envoycachetypes.Endpoint])

	g.Expect(cdsNames).ToNot(gomega.ContainElement(reproBrokenCluster),
		"errored cluster is correctly withheld from CDS")

	// Without the errored-cluster filtering this CLA leaked into EDS, and in ADS
	// mode one resource Envoy was never given poisons the whole EDS response.
	g.Expect(edsNames).ToNot(gomega.ContainElement(reproBrokenCluster),
		"CLA for the errored cluster must not be published: Envoy never received the cluster, "+
			"so it will not list that CLA in its EDS request, and go-control-plane's ADS superset "+
			"check then withholds every other cluster's endpoints")

	// Do not generalize this to require every EDS resource to appear in dynamic
	// CDS. For example, Envoy's bootstrap defines the local EDS cluster, while
	// the control plane serves its CLA dynamically without a matching cluster in
	// CDS. The invariant here is specifically that an errored cluster omitted
	// from CDS must not leave its own CLA behind.
}

// TestEndpointUpdatesFlowWhileAnotherClusterErrored drives the published
// snapshots through the real ADS server and snapshot cache, with a client that
// subscribes the way Envoy does: EDS resource names come from the clusters CDS
// actually delivered. Before the errored-cluster CLA filtering, the customer
// symptom appeared in phase 3 - after one BackendConfigPolicy was rejected,
// endpoint updates for unrelated clusters never arrived again. Phase 4 pins the
// version handling: recovery must re-deliver the CLA even when the endpoints
// themselves never changed.
func TestEndpointUpdatesFlowWhileAnotherClusterErrored(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	f := newErroredClusterFixture(t)
	logs := &capturingXdsLogger{}
	// Same cache and server construction as setup/controlplane.go: ADS mode,
	// node-role hasher, unordered ADS (EnableOrderedAds defaults to false).
	cache := envoycache.NewSnapshotCache(true, xds.NewNodeRoleHasher(), logs)
	envoy := startADSServerAndClient(t, ctx, cache, f.ucc.ResourceName())

	// MARK: phase 1 - both BackendConfigPolicies are valid
	snap := f.awaitSnapshot(g, func(s XdsSnapWrapper) bool { return len(s.erroredClusters) == 0 })
	f.setSnapshot(g, ctx, cache, snap)

	clusters := envoy.syncClusters(g)
	g.Expect(clusters).To(gomega.ConsistOf(reproHealthyCluster, reproBrokenCluster))

	claNames := envoy.syncEndpoints(g)
	g.Expect(claNames).To(gomega.ConsistOf(reproHealthyCluster, reproBrokenCluster),
		"healthy state must deliver endpoints for both clusters")

	// MARK: phase 2 - echo-server's policy is patched with a bogus cipher suite
	f.breakCluster(reproBrokenCluster)
	snap = f.awaitSnapshot(g, func(s XdsSnapWrapper) bool { return len(s.erroredClusters) == 1 })
	f.setSnapshot(g, ctx, cache, snap)

	// Envoy is pushed the new CDS, sees echo-server's cluster removed, and
	// narrows its EDS subscription to the clusters it still has.
	clusters = envoy.awaitClusters(g)
	g.Expect(clusters).To(gomega.ConsistOf(reproHealthyCluster),
		"errored cluster is withheld from CDS, so Envoy stops subscribing to its endpoints")
	claNames = envoy.syncEndpoints(g)
	g.Expect(claNames).To(gomega.ConsistOf(reproHealthyCluster),
		"EDS must immediately withdraw the errored cluster's CLA without withholding healthy endpoints")

	// MARK: phase 3 - the healthy Service scales; its endpoints must still flow
	f.updateEndpoints(reproHealthyCluster, 2)
	snap = f.awaitSnapshot(g, func(s XdsSnapWrapper) bool {
		cla := snapshotCLA(s, reproHealthyCluster)
		return cla != nil && len(cla.GetEndpoints()) == 2
	})
	f.setSnapshot(g, ctx, cache, snap)

	resp := envoy.await(envoyresource.EndpointType, 3*time.Second)
	g.Expect(resp).ToNot(gomega.BeNil(),
		fmt.Sprintf("endpoint update for the healthy cluster never reached Envoy: the control plane "+
			"withheld the EDS response and dropped the watch, so this proxy is frozen on stale "+
			"endpoints until it reconnects. control-plane warnings: %v", logs.warnings()))
	g.Expect(localityCount(g, resp, reproHealthyCluster)).To(gomega.Equal(2),
		"Envoy must see the scaled-up endpoints of the healthy cluster")

	// MARK: phase 4 - fixing the policy restores the cluster and its endpoints
	f.restoreCluster(reproBrokenCluster)
	snap = f.awaitSnapshot(g, func(s XdsSnapWrapper) bool { return len(s.erroredClusters) == 0 })
	f.setSnapshot(g, ctx, cache, snap)

	clusters = envoy.awaitClusters(g)
	g.Expect(clusters).To(gomega.ConsistOf(reproHealthyCluster, reproBrokenCluster),
		"the recovered cluster must return to CDS")
	claNames = envoy.syncEndpoints(g)
	g.Expect(claNames).To(gomega.ConsistOf(reproHealthyCluster, reproBrokenCluster),
		"the recovered cluster's CLA must be sent even when its endpoint content did not change")
}

// MARK: fixture

// erroredClusterFixture wires snapshotPerClient over static collections standing
// in for the per-client cluster and endpoint collections.
type erroredClusterFixture struct {
	ucc         ir.UniquelyConnectedClient
	clusterCols *testClusterCols
	endpointCol krt.StaticCollection[UccWithEndpoints]
	snapshots   krt.Collection[XdsSnapWrapper]
}

func newErroredClusterFixture(t *testing.T) *erroredClusterFixture {
	t.Helper()

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "jwt-test", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "jwt-test", Name: "gw"},
		Listeners:      sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}}),
		Routes: sliceToResources([]*envoyroutev3.RouteConfiguration{{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{{
				Name:    "vhost",
				Domains: []string{"*"},
				Routes: []*envoyroutev3.Route{
					routeToCluster("healthy-route", reproHealthyCluster),
					routeToCluster("echo-server-route", reproBrokenCluster),
				},
			}},
		}}),
	}})

	f := &erroredClusterFixture{ucc: ucc}

	// Neither cluster varies per client, so both are resolved shared bases with
	// no overlay; breakCluster/restoreCluster swap the base row in place.
	pcc, cols := newTestPerClientClusters([]uccWithCluster{
		edsCluster(ucc, reproHealthyCluster, 1),
		edsCluster(ucc, reproBrokenCluster, 2),
	})
	f.clusterCols = cols
	f.endpointCol = krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		f.endpointsFor(reproHealthyCluster, 1),
		f.endpointsFor(reproBrokenCluster, 1),
	})

	f.snapshots = snapshotPerClient(
		krtutil.KrtOptions{},
		uccs,
		mostXdsSnapshots,
		PerClientEnvoyEndpoints{
			endpoints: f.endpointCol,
			index: krtpkg.UnnamedIndex(f.endpointCol, func(ep UccWithEndpoints) []string {
				return []string{ep.Client.ResourceName()}
			}),
		},
		pcc,
	)

	return f
}

// breakCluster marks a cluster as failing translation, which is what a
// BackendConfigPolicy rejected by STRICT validation produces. The CLA is
// deliberately left in place: the endpoint collection is keyed off the backend,
// not the cluster's error state.
func (f *erroredClusterFixture) breakCluster(name string) {
	c := edsCluster(f.ucc, name, 99)
	c.Error = errors.New(reproValidationErr)
	f.clusterCols.bases.UpdateObject(baseFromCluster(c))
}

func (f *erroredClusterFixture) restoreCluster(name string) {
	f.clusterCols.bases.UpdateObject(baseFromCluster(edsCluster(f.ucc, name, 100)))
}

func (f *erroredClusterFixture) updateEndpoints(name string, localities int) {
	f.endpointCol.UpdateObject(f.endpointsFor(name, localities))
}

func (f *erroredClusterFixture) endpointsFor(name string, localities int) UccWithEndpoints {
	// Distinct hash per (cluster, endpoint count) so the endpoint snapshot
	// version moves when endpoints change; endpointsWithUccName.Equals compares
	// that version.
	hash := utils.HashString(fmt.Sprintf("%s/%d", name, localities))

	lbEps := make([]*envoyendpointv3.LocalityLbEndpoints, 0, localities)
	for i := range localities {
		lbEps = append(lbEps, &envoyendpointv3.LocalityLbEndpoints{
			Locality: &envoycorev3.Locality{Zone: fmt.Sprintf("zone-%d", i)},
		})
	}

	return UccWithEndpoints{
		Client:        f.ucc,
		Endpoints:     &envoyendpointv3.ClusterLoadAssignment{ClusterName: name, Endpoints: lbEps},
		EndpointsHash: hash,
		endpointsName: name,
	}
}

func (f *erroredClusterFixture) awaitSnapshot(g gomega.Gomega, pred func(XdsSnapWrapper) bool) XdsSnapWrapper {
	var found XdsSnapWrapper
	g.Eventually(func() bool {
		for _, s := range f.snapshots.List() {
			if pred(s) {
				found = s
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond).Should(gomega.BeTrue(), "expected snapshot was never published")
	return found
}

// setSnapshot mirrors ProxyTranslator.syncXds.
func (f *erroredClusterFixture) setSnapshot(
	g gomega.Gomega,
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snap XdsSnapWrapper,
) {
	g.Expect(cache.SetSnapshot(ctx, snap.proxyKey, snap.snap)).To(gomega.Succeed())
}

// MARK: fake envoy

// fakeEnvoy is an ADS client that models the parts of Envoy's behavior that
// matter here: CDS is a wildcard subscription, and the EDS subscription is
// exactly the set of EDS clusters CDS delivered. Every response is ACKed, which
// is what leaves a pending watch in the cache for the next push.
type fakeEnvoy struct {
	stream  envoy_service_discovery_v3.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	node    *envoycorev3.Node
	inbound chan *envoy_service_discovery_v3.DiscoveryResponse
	stash   map[string][]*envoy_service_discovery_v3.DiscoveryResponse

	version  map[string]string
	nonce    map[string]string
	subs     map[string][]string
	recvErr  chan error
	edsNames []string
}

func startADSServerAndClient(
	t *testing.T,
	ctx context.Context,
	cache envoycache.SnapshotCache,
	nodeID string,
) *fakeEnvoy {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsserver.NewServer(ctx, cache, nil))
	go grpcServer.Serve(lis) //nolint:errcheck // test server
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	stream, err := envoy_service_discovery_v3.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
	if err != nil {
		t.Fatalf("open ADS stream: %v", err)
	}

	e := &fakeEnvoy{
		stream: stream,
		node: &envoycorev3.Node{
			Id: "envoy",
			Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
				xds.RoleKey: structpb.NewStringValue(nodeID),
			}},
		},
		inbound: make(chan *envoy_service_discovery_v3.DiscoveryResponse, 32),
		stash:   map[string][]*envoy_service_discovery_v3.DiscoveryResponse{},
		version: map[string]string{},
		nonce:   map[string]string{},
		subs:    map[string][]string{},
		recvErr: make(chan error, 1),
	}
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				select {
				case e.recvErr <- err:
				default:
				}
				return
			}
			e.inbound <- resp
		}
	}()

	return e
}

// request sends a subscription request for a type, carrying the last accepted
// version and nonce so the server does not discard it as stale.
func (e *fakeEnvoy) request(typeURL string, names []string) {
	e.subs[typeURL] = names
	//nolint:errcheck // a send failure surfaces as a missing response
	e.stream.Send(&envoy_service_discovery_v3.DiscoveryRequest{
		Node:          e.node,
		TypeUrl:       typeURL,
		ResourceNames: names,
		VersionInfo:   e.version[typeURL],
		ResponseNonce: e.nonce[typeURL],
	})
}

// await returns the next response for a type, ACKing it the way Envoy does, or
// nil if none arrives before the timeout.
func (e *fakeEnvoy) await(typeURL string, timeout time.Duration) *envoy_service_discovery_v3.DiscoveryResponse {
	if queued := e.stash[typeURL]; len(queued) > 0 {
		e.stash[typeURL] = queued[1:]
		return e.ack(queued[0])
	}
	deadline := time.After(timeout)
	for {
		select {
		case resp := <-e.inbound:
			if resp.GetTypeUrl() == typeURL {
				return e.ack(resp)
			}
			e.stash[resp.GetTypeUrl()] = append(e.stash[resp.GetTypeUrl()], resp)
		case <-e.recvErr:
			return nil
		case <-deadline:
			return nil
		}
	}
}

// ack records the version/nonce and re-sends the request, which is what makes
// the cache register a pending watch for the next snapshot.
func (e *fakeEnvoy) ack(resp *envoy_service_discovery_v3.DiscoveryResponse) *envoy_service_discovery_v3.DiscoveryResponse {
	e.version[resp.GetTypeUrl()] = resp.GetVersionInfo()
	e.nonce[resp.GetTypeUrl()] = resp.GetNonce()
	e.request(resp.GetTypeUrl(), e.subs[resp.GetTypeUrl()])
	return resp
}

// syncClusters subscribes to CDS (wildcard) and returns the cluster names Envoy
// now has, recording the EDS subscription those clusters imply.
func (e *fakeEnvoy) syncClusters(g gomega.Gomega) []string {
	e.request(envoyresource.ClusterType, nil)
	return e.awaitClusters(g)
}

// awaitClusters consumes a CDS push (no new request needed: the ACK of the
// previous response already left a watch open).
func (e *fakeEnvoy) awaitClusters(g gomega.Gomega) []string {
	resp := e.await(envoyresource.ClusterType, 5*time.Second)
	g.Expect(resp).ToNot(gomega.BeNil(), "CDS response was withheld")

	var names, edsNames []string
	for _, res := range resp.GetResources() {
		var c envoyclusterv3.Cluster
		g.Expect(res.UnmarshalTo(&c)).To(gomega.Succeed())
		names = append(names, c.GetName())
		if c.GetType() == envoyclusterv3.Cluster_EDS {
			// Envoy subscribes to the EDS resource of every EDS cluster it has,
			// and only to those: clusters removed from CDS are removed from
			// Envoy, dropping their EDS subscription with them.
			serviceName := c.GetEdsClusterConfig().GetServiceName()
			if serviceName == "" {
				serviceName = c.GetName()
			}
			edsNames = append(edsNames, serviceName)
		}
	}
	e.edsNames = edsNames
	return names
}

// syncEndpoints subscribes to EDS for the current cluster set and returns the
// CLA names delivered.
func (e *fakeEnvoy) syncEndpoints(g gomega.Gomega) []string {
	e.resubscribeEndpoints()
	resp := e.await(envoyresource.EndpointType, 5*time.Second)
	g.Expect(resp).ToNot(gomega.BeNil(), "EDS response was withheld")

	var names []string
	for _, res := range resp.GetResources() {
		var cla envoyendpointv3.ClusterLoadAssignment
		g.Expect(res.UnmarshalTo(&cla)).To(gomega.Succeed())
		names = append(names, cla.GetClusterName())
	}
	return names
}

func (e *fakeEnvoy) resubscribeEndpoints() {
	e.request(envoyresource.EndpointType, e.edsNames)
}

// MARK: helpers

func localityCount(g gomega.Gomega, resp *envoy_service_discovery_v3.DiscoveryResponse, cluster string) int {
	for _, res := range resp.GetResources() {
		var cla envoyendpointv3.ClusterLoadAssignment
		g.Expect(res.UnmarshalTo(&cla)).To(gomega.Succeed())
		if cla.GetClusterName() == cluster {
			return len(cla.GetEndpoints())
		}
	}
	return 0
}

func snapshotCLA(s XdsSnapWrapper, name string) *envoyendpointv3.ClusterLoadAssignment {
	item, ok := s.snap.Resources[envoycachetypes.Endpoint].Items[name]
	if !ok {
		return nil
	}
	cla, _ := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
	return cla
}

func resourceNames(res envoycache.Resources) []string {
	names := make([]string, 0, len(res.Items))
	for name := range res.Items {
		names = append(names, name)
	}
	return names
}

func edsCluster(ucc ir.UniquelyConnectedClient, name string, version uint64) uccWithCluster {
	return uccWithCluster{
		Client: ucc,
		Name:   name,
		Cluster: sharedproto.Wrap(&envoyclusterv3.Cluster{
			Name:                 name,
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
			EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
				ServiceName: name,
				EdsConfig: &envoycorev3.ConfigSource{
					ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}},
				},
			},
		}),
		ClusterVersion: version,
	}
}

func routeToCluster(name, cluster string) *envoyroutev3.Route {
	return &envoyroutev3.Route{
		Name: name,
		Action: &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: cluster},
			},
		},
	}
}

// capturingXdsLogger records the snapshot cache's warnings so a failing
// assertion can quote the "not listed" line the customer saw.
type capturingXdsLogger struct {
	mu   sync.Mutex
	warn []string
}

func (l *capturingXdsLogger) Debugf(string, ...any) {}
func (l *capturingXdsLogger) Infof(string, ...any)  {}

func (l *capturingXdsLogger) Warnf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warn = append(l.warn, fmt.Sprintf(format, args...))
}

func (l *capturingXdsLogger) Errorf(format string, args ...any) {
	l.Warnf(format, args...)
}

func (l *capturingXdsLogger) warnings() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.warn) == 0 {
		return "(none)"
	}
	return strings.Join(l.warn, "; ")
}

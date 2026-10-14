package proxy_syncer

import (
	"context"
	"strings"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoygenericsecretv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/generic_secret/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// tunnelCluster is a tunneled backend's cluster as the CONNECT tunnel
// generates it: STATIC, with one endpoint on the pipe its listener binds.
func tunnelCluster(name string) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC},
		LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{LbEndpoints: []*envoyendpointv3.LbEndpoint{{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: "@" + name}}},
				}},
			}}}},
		},
	}
}

// tunnelBoundListener binds the pipe tunnelCluster(cluster) reaches and
// CONNECTs through proxy, adding a header read from the SDS secret secret.
func tunnelBoundListener(cluster, proxy, secret string) *envoylistenerv3.Listener {
	tcp := &envoytcpv3.TcpProxy{
		StatPrefix:       "tunnel_" + cluster,
		ClusterSpecifier: &envoytcpv3.TcpProxy_Cluster{Cluster: proxy},
		TunnelingConfig: &envoytcpv3.TcpProxy_TunnelingConfig{
			Hostname: "dest.example.com:443",
			HeadersToAdd: []*envoycorev3.HeaderValueOption{{
				Header: &envoycorev3.HeaderValue{Key: "proxy-authorization", Value: "%SECRET(" + secret + ")%"},
			}},
			Formatters: []*envoycorev3.TypedExtensionConfig{{
				Name: "envoy.formatter.generic_secret",
				TypedConfig: utils.MustMessageToAny(&envoygenericsecretv3.GenericSecret{
					SecretConfigs: map[string]*envoytlsv3.SdsSecretConfig{secret: {
						Name:      secret,
						SdsConfig: &envoycorev3.ConfigSource{ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}}},
					}},
				}),
			}},
		},
	}
	return &envoylistenerv3.Listener{
		Name:    "tunnel_" + cluster,
		Address: &envoycorev3.Address{Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: "@" + cluster}}},
		FilterChains: []*envoylistenerv3.FilterChain{{Filters: []*envoylistenerv3.Filter{{
			Name:       envoywellknown.TCPProxy,
			ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: utils.MustMessageToAny(tcp)},
		}}}},
	}
}

func genericSecret(name, value string) *envoytlsv3.Secret {
	return &envoytlsv3.Secret{Name: name, Type: &envoytlsv3.Secret_GenericSecret{GenericSecret: &envoytlsv3.GenericSecret{
		Secret: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: value}},
	}}}
}

// tunnelBuild is one build for a gateway routing to "x", a backend tunneled
// through "proxy". withTunnel is whether x's row (its cluster, listener and
// secret) made the build; withProxy whether proxy's did.
type tunnelBuild struct {
	version     string
	withTunnel  bool
	withProxy   bool
	secretValue string
	// proxyStatName marks a proxy cluster's content, to tell copies apart.
	proxyStatName string
}

func (b tunnelBuild) snapshot(t *testing.T) *envoycache.Snapshot {
	t.Helper()
	ucc := newTestUcc(publishGateTestClient)
	listeners := []envoycachetypes.Resource{httpListenerWithRDS(t, "listener", "route-config")}
	var clusters, endpoints, secrets []envoycachetypes.Resource
	if b.withTunnel {
		listeners = append(listeners, tunnelBoundListener("x", "proxy", "tunnel_x/h"))
		clusters = append(clusters, tunnelCluster("x"))
		secrets = append(secrets, genericSecret("tunnel_x/h", b.secretValue))
	}
	if b.withProxy {
		proxy := proto.Clone(edsClusterForClient(ucc, "proxy", 1).Cluster.BorrowForRead()).(*envoyclusterv3.Cluster)
		proxy.AltStatName = b.proxyStatName
		clusters = append(clusters, proxy)
		endpoints = append(endpoints, endpointsForClient(ucc, "proxy", 2).Endpoints.BorrowForRead())
	}
	resources := map[envoyresourcev3.Type][]envoycachetypes.Resource{
		envoyresourcev3.ListenerType: listeners,
		envoyresourcev3.ClusterType:  clusters,
		envoyresourcev3.EndpointType: endpoints,
		envoyresourcev3.SecretType:   secrets,
	}
	snap, err := envoycache.NewSnapshot(b.version, resources)
	require.NoError(t, err)
	routes := routeResourcesForClusters("x")
	routes.Version = "rds-" + b.version
	snap.Resources[envoycachetypes.Route] = routes
	return snap
}

func (b tunnelBuild) wrapper(t *testing.T, missing ...string) XdsSnapWrapper {
	return XdsSnapWrapper{snap: b.snapshot(t), proxyKey: publishGateTestClient, missingReferenced: missing}
}

// TestCarriedTunnelKeepsWhatItNeeds: when a tunneled backend's row is
// missing and the gate carries its cluster, the tunnel's listener, its header
// secret and, if the build lacks it (as with scoping), the proxy cluster and
// its CLA are carried with it. Each type's version moves off the build's.
func TestCarriedTunnelKeepsWhatItNeeds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		withProxy bool
	}{
		{name: "proxy still built", withProxy: true},
		{name: "proxy dropped with the tunnel's row", withProxy: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pt := newPublishGateTestTranslator(t, false, time.Hour)
			t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
			ctx := context.Background()

			pt.syncXds(ctx, tunnelBuild{version: "v1", withTunnel: true, withProxy: true, secretValue: "s1"}.wrapper(t))
			lagging := tunnelBuild{version: "v2", withProxy: tc.withProxy}
			pt.syncXds(ctx, lagging.wrapper(t, "x"))

			served := servedSnapshot(t, pt)
			assert.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "x"), "the tunneled cluster is carried")
			assert.True(t, hasResource(served.Resources[envoycachetypes.Listener].Items, "tunnel_x"),
				"the listener on the carried cluster's pipe is carried with it")
			assert.True(t, hasResource(served.Resources[envoycachetypes.Secret].Items, "tunnel_x/h"),
				"the secret the tunnel's listener reads is carried with it")
			assert.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "proxy"),
				"the cluster the tunnel's listener proxies through is present")
			_, hasCLA := endpointCount(served, "proxy")
			assert.True(t, hasCLA, "and its CLA")

			build := lagging.snapshot(t)
			assert.True(t, strings.HasPrefix(served.GetVersion(envoyresourcev3.ListenerType), build.GetVersion(envoyresourcev3.ListenerType)+"-carry-"))
			assert.True(t, strings.HasPrefix(served.GetVersion(envoyresourcev3.SecretType), build.GetVersion(envoyresourcev3.SecretType)+"-carry-"))
			assert.Equal(t, "rds-v2", served.GetVersion(envoyresourcev3.RouteType), "the build's routes still publish")
		})
	}
}

// TestCarryKeepsBuiltDependencies: a dependency the build has is kept as
// built; the published copy never replaces current configuration or a
// rotated secret.
func TestCarryKeepsBuiltDependencies(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, tunnelBuild{version: "v1", withTunnel: true, withProxy: true, secretValue: "old", proxyStatName: "old"}.wrapper(t))
	lagging := tunnelBuild{version: "v2", withProxy: true, proxyStatName: "new"}.snapshot(t)
	// The secret is still built, rotated, by another of its users.
	lagging.Resources[envoycachetypes.Secret] = envoycache.NewResources("sds-v2", []envoycachetypes.Resource{genericSecret("tunnel_x/h", "new")})
	pt.syncXds(ctx, XdsSnapWrapper{snap: lagging, proxyKey: publishGateTestClient, missingReferenced: []string{"x"}})

	served := servedSnapshot(t, pt)
	require.True(t, hasResource(served.Resources[envoycachetypes.Listener].Items, "tunnel_x"))
	proxy, ok := served.Resources[envoycachetypes.Cluster].Items["proxy"].Resource.(*envoyclusterv3.Cluster)
	require.True(t, ok)
	assert.Equal(t, "new", proxy.GetAltStatName(), "the built proxy cluster is kept")
	secret, ok := served.Resources[envoycachetypes.Secret].Items["tunnel_x/h"].Resource.(*envoytlsv3.Secret)
	require.True(t, ok)
	assert.Equal(t, "new", secret.GetGenericSecret().GetSecret().GetInlineString(), "the rotated secret is kept")
	assert.Equal(t, "sds-v2", served.GetVersion(envoyresourcev3.SecretType), "nothing was carried into SDS")
}

// TestCarryDoesNotReviveAnErroredDependency: a cluster the tunnel's listener
// routes to that errored in the build is not carried back, like any errored
// cluster.
func TestCarryDoesNotReviveAnErroredDependency(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, tunnelBuild{version: "v1", withTunnel: true, withProxy: true, secretValue: "s1"}.wrapper(t))
	lagging := tunnelBuild{version: "v2"}.wrapper(t, "x")
	lagging.erroredClusters = []string{"proxy"}
	pt.syncXds(ctx, lagging)

	served := servedSnapshot(t, pt)
	assert.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "x"))
	assert.False(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "proxy"), "an errored proxy is not carried")
	_, hasCLA := endpointCount(served, "proxy")
	assert.False(t, hasCLA)
}

// TestCarryDependenciesEndWithTheCarry: past the warm withhold bound the
// tunneled cluster is no longer carried, and neither is anything carried for
// it. A recovered build publishes its own resources, with the build's versions.
func TestCarryDependenciesEndWithTheCarry(t *testing.T) {
	ResetMetrics()
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.gate.warmWithholdBound = 100 * time.Millisecond
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, tunnelBuild{version: "v1", withTunnel: true, withProxy: true, secretValue: "s1"}.wrapper(t))
	pt.syncXds(ctx, tunnelBuild{version: "v2"}.wrapper(t, "x"))
	require.True(t, hasResource(servedSnapshot(t, pt).Resources[envoycachetypes.Listener].Items, "tunnel_x"))

	time.Sleep(150 * time.Millisecond)
	pt.syncXds(ctx, tunnelBuild{version: "v3"}.wrapper(t, "x"))
	served := servedSnapshot(t, pt)
	for rt, name := range map[envoycachetypes.ResponseType]string{
		envoycachetypes.Cluster:  "x",
		envoycachetypes.Listener: "tunnel_x",
		envoycachetypes.Secret:   "tunnel_x/h",
	} {
		assert.False(t, hasResource(served.Resources[rt].Items, name), "%s is not carried past the bound", name)
	}
	assert.False(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "proxy"), "nor the proxy it pulled in")

	recovered := tunnelBuild{version: "v4", withTunnel: true, withProxy: true, secretValue: "s2"}
	pt.syncXds(ctx, recovered.wrapper(t))
	served = servedSnapshot(t, pt)
	assert.Equal(t, "v4", served.GetVersion(envoyresourcev3.ListenerType))
	assert.Equal(t, "v4", served.GetVersion(envoyresourcev3.SecretType))
}

// TestCarryDependencyCyclesEnd: listeners that route between carried clusters
// in a cycle are each carried once.
func TestCarryDependencyCyclesEnd(t *testing.T) {
	published, err := envoycache.NewSnapshot("v1", map[envoyresourcev3.Type][]envoycachetypes.Resource{
		envoyresourcev3.ClusterType:  {tunnelCluster("x"), tunnelCluster("y")},
		envoyresourcev3.ListenerType: {httpListenerWithRDS(t, "listener", "route-config"), tunnelBoundListener("x", "y", "sx"), tunnelBoundListener("y", "x", "sy")},
		envoyresourcev3.SecretType:   {genericSecret("sx", "1"), genericSecret("sy", "2")},
	})
	require.NoError(t, err)
	published.Resources[envoycachetypes.Route] = routeResourcesForClusters("x")
	build, err := envoycache.NewSnapshot("v2", map[envoyresourcev3.Type][]envoycachetypes.Resource{
		envoyresourcev3.ListenerType: {httpListenerWithRDS(t, "listener", "route-config")},
	})
	require.NoError(t, err)
	build.Resources[envoycachetypes.Route] = routeResourcesForClusters("x")

	composed, _, _ := resolveDeferredPerCluster(XdsSnapWrapper{snap: build, proxyKey: publishGateTestClient, missingReferenced: []string{"x"}}, published, true,
		func(string) (uint64, bool) { return 0, false })

	assert.ElementsMatch(t, []string{"x", "y"}, mapKeys(composed.Resources[envoycachetypes.Cluster].Items))
	assert.ElementsMatch(t, []string{"listener", "tunnel_x", "tunnel_y"}, mapKeys(composed.Resources[envoycachetypes.Listener].Items))
	assert.ElementsMatch(t, []string{"sx", "sy"}, mapKeys(composed.Resources[envoycachetypes.Secret].Items))
}

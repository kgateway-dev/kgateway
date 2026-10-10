package backendconfigpolicy

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"slices"
	"strconv"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoygenericsecret "github.com/envoyproxy/go-control-plane/envoy/extensions/formatter/generic_secret/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// connectTunnelPrefix starts the names of a tunnel's listener, socket, and secrets.
const connectTunnelPrefix = "connect_tunnel_"

// newTunnelHook generates CONNECT listeners, resolving proxies after policy
// attachment to avoid a KRT dependency cycle.
// BackendIndex is initialized after plugin construction.
func newTunnelHook(commoncol *collections.CommonCollections) sdk.ProcessBaseClusterResources {
	return func(kctx krt.HandlerContext, _ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) (sdk.BaseClusterResources, error) {
		tunnel := effectiveTunnel(in)
		if tunnel == nil {
			return sdk.BaseClusterResources{}, nil
		}
		resources, err := translateTunnelResources(kctx, commoncol.BackendIndex, in, out, tunnel)
		resources.Policy = &ir.AttachedPolicyRef{
			Group:     tunnel.source.Group,
			Kind:      tunnel.source.Kind,
			Namespace: tunnel.source.Namespace,
			Name:      tunnel.source.Name,
		}
		return resources, err
	}
}

// translateTunnelResources validates the tunnel, rewrites the destination
// cluster, and returns its CONNECT listener and header secrets.
func translateTunnelResources(
	kctx krt.HandlerContext,
	backends *krtcollections.BackendIndex,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
	tunnel *tunnelIR,
) (sdk.BaseClusterResources, error) {
	target, err := tunnelDestination(in)
	if err != nil {
		return sdk.BaseClusterResources{}, err
	}
	if err := validateTunnelCluster(out); err != nil {
		return sdk.BaseClusterResources{}, err
	}
	proxyCluster, err := resolveTunnelProxy(kctx, backends, tunnel)
	if err != nil {
		return sdk.BaseClusterResources{}, err
	}
	name := connectTunnelPrefix + out.GetName()
	// @ is Envoy's prefix for a Linux abstract Unix socket. Its path is limited
	// to 108 bytes, so it hashes the cluster name.
	socket := "@" + connectTunnelPrefix + strconv.FormatUint(utils.HashString(out.GetName()), 16)
	listener, secrets := buildTunnelListener(name, socket, proxyCluster, target, tunnel.headers)
	rewriteTunnelCluster(out, target, socket)
	return sdk.BaseClusterResources{Listeners: []*envoylistenerv3.Listener{listener}, Secrets: secrets}, nil
}

// effectiveTunnel returns the tunnel selected by policy precedence.
// Errored policies fail base translation before resource hooks run.
func effectiveTunnel(in ir.BackendObjectIR) *tunnelIR {
	policies := in.AttachedPolicies.Policies[wellknown.BackendConfigPolicyGVK.GroupKind()]
	setsTunnel := func(att ir.PolicyAtt) bool {
		pol, ok := att.PolicyIr.(*BackendConfigPolicyIR)
		return ok && pol.tunnel != nil
	}
	if !slices.ContainsFunc(policies, setsTunnel) {
		return nil
	}
	merged, ok := mergePolicies(policies).PolicyIr.(*BackendConfigPolicyIR)
	if !ok {
		return nil
	}
	return merged.tunnel
}

// tunnelDestination returns the host and port of a single-host Static Backend.
func tunnelDestination(in ir.BackendObjectIR) (kgateway.Host, error) {
	backend, ok := in.Obj.(*kgateway.Backend)
	if !ok || backend.Spec.Static == nil {
		return kgateway.Host{}, errors.New("tunnel: only Static Backends can be tunneled")
	}
	hosts := backend.Spec.Static.Hosts
	if len(hosts) != 1 {
		return kgateway.Host{}, fmt.Errorf("tunnel: a tunneled Static Backend must have exactly one host, found %d", len(hosts))
	}
	return hosts[0], nil
}

// validateTunnelCluster rejects unsupported destination settings; connections
// to the generated listener use a Unix socket.
func validateTunnelCluster(out *envoyclusterv3.Cluster) error {
	switch {
	case len(out.GetHealthChecks()) > 0:
		return errors.New("tunnel: active health checks are not supported on a tunneled backend")
	case out.GetUpstreamConnectionOptions().GetTcpKeepalive() != nil:
		return errors.New("tunnel: TCP keepalive is not supported on a tunneled backend")
	case out.GetUpstreamBindConfig() != nil:
		return errors.New("tunnel: a source address or socket options are not supported on a tunneled backend")
	case out.GetTransportSocket().GetName() == wellknown.TransportSocketUpstreamProxyProtocol:
		return errors.New("tunnel: upstream PROXY protocol is not supported on a tunneled backend")
	}
	return nil
}

// resolveTunnelProxy validates the proxy backend and returns its cluster name.
func resolveTunnelProxy(kctx krt.HandlerContext, backends *krtcollections.BackendIndex, tunnel *tunnelIR) (string, error) {
	ref := fmt.Sprintf("%s/%s", ptr.Deref(tunnel.proxy.Namespace, gwv1.Namespace(tunnel.source.Namespace)), tunnel.proxy.Name)
	proxy, err := backends.GetBackendFromRef(kctx, tunnel.source, tunnel.proxy)
	if err != nil {
		return "", fmt.Errorf("tunnel proxy %s: %w", ref, err)
	}
	if b, ok := proxy.Obj.(*kgateway.Backend); ok && b.Spec.Static == nil {
		return "", fmt.Errorf("tunnel proxy %s: only Static Backends can be proxies", ref)
	}
	if len(proxy.Errors) > 0 {
		return "", fmt.Errorf("tunnel proxy %s: %w", ref, errors.Join(proxy.Errors...))
	}
	if effectiveTunnel(*proxy) != nil {
		return "", fmt.Errorf("tunnel proxy %s is itself tunneled", ref)
	}
	return proxy.ClusterName(), nil
}

// rewriteTunnelCluster replaces discovery with a pipe endpoint, preserving
// destination TLS and the hostname used by autoHostRewrite.
func rewriteTunnelCluster(out *envoyclusterv3.Cluster, target kgateway.Host, socket string) {
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}
	out.EdsClusterConfig = nil
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
			LbEndpoints: []*envoyendpointv3.LbEndpoint{{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Hostname: target.Host,
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: socket}},
						},
					},
				},
			}},
		}},
	}
}

// buildTunnelListener creates a Unix listener that CONNECTs through the proxy
// cluster. Header values are returned as SDS secrets, which the listener reads
// with Envoy's generic_secret formatter, so LDS carries no credentials.
func buildTunnelListener(
	name, socket, proxyCluster string,
	target kgateway.Host,
	headers []gwv1.HTTPHeader,
) (*envoylistenerv3.Listener, []*envoytlsv3.Secret) {
	authority := net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port)))
	tunneling := &envoytcp.TcpProxy_TunnelingConfig{
		Hostname: strings.ReplaceAll(authority, "%", "%%"),
	}
	var secrets []*envoytlsv3.Secret
	secretConfigs := map[string]*envoytlsv3.SdsSecretConfig{}
	for i, h := range headers {
		// Envoy caches SDS values by name, and LDS and SDS updates arrive apart.
		// Rotation keeps the name; a new proxy or header renames it, so a listener
		// never sends a value cached for another proxy or header.
		hasher := fnv.New64a()
		utils.HashStringField(hasher, proxyCluster)
		utils.HashStringField(hasher, strconv.Itoa(i))
		utils.HashStringField(hasher, string(h.Name))
		secretName := name + "/" + strconv.FormatUint(hasher.Sum64(), 16)
		tunneling.HeadersToAdd = append(tunneling.HeadersToAdd, &envoycorev3.HeaderValueOption{
			Header:       &envoycorev3.HeaderValue{Key: string(h.Name), Value: "%SECRET(" + secretName + ")%"},
			AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
		secretConfigs[secretName] = &envoytlsv3.SdsSecretConfig{
			Name: secretName,
			SdsConfig: &envoycorev3.ConfigSource{
				ResourceApiVersion:    envoycorev3.ApiVersion_V3,
				ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}},
			},
		}
		secrets = append(secrets, &envoytlsv3.Secret{
			Name: secretName,
			Type: &envoytlsv3.Secret_GenericSecret{GenericSecret: &envoytlsv3.GenericSecret{
				Secret: pluginutils.InlineStringDataSource(h.Value),
			}},
		})
	}
	if len(headers) > 0 {
		tunneling.Formatters = []*envoycorev3.TypedExtensionConfig{{
			Name:        "envoy.formatter.generic_secret",
			TypedConfig: utils.MustMessageToAny(&envoygenericsecret.GenericSecret{SecretConfigs: secretConfigs}),
		}}
	}
	config := utils.MustMessageToAny(&envoytcp.TcpProxy{
		StatPrefix:       name,
		ClusterSpecifier: &envoytcp.TcpProxy_Cluster{Cluster: proxyCluster},
		TunnelingConfig:  tunneling,
	})
	return &envoylistenerv3.Listener{
		Name:       name,
		StatPrefix: name,
		Address: &envoycorev3.Address{
			Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: socket}},
		},
		FilterChains: []*envoylistenerv3.FilterChain{{
			Filters: []*envoylistenerv3.Filter{{
				Name:       envoywellknown.TCPProxy,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: config},
			}},
		}},
	}, secrets
}

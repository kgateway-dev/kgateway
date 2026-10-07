package backendconfigpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type tunnelTarget struct {
	host string
	port uint32
}

func (t tunnelTarget) authority() string {
	return net.JoinHostPort(t.host, strconv.FormatUint(uint64(t.port), 10))
}

// newTunnelHook generates CONNECT listeners, resolving proxies after policy
// attachment to avoid a KRT dependency cycle.
// BackendIndex is initialized after plugin construction.
func newTunnelHook(commoncol *collections.CommonCollections) sdk.ProcessBaseClusterResources {
	return func(kctx krt.HandlerContext, _ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) (sdk.BaseClusterResources, error) {
		tunnel := effectiveTunnel(in)
		if tunnel == nil {
			return sdk.BaseClusterResources{}, nil
		}
		listener, err := translateTunnelResources(kctx, commoncol.BackendIndex, in, out, tunnel)
		if err != nil {
			return sdk.BaseClusterResources{}, &ir.PolicyError{Ref: tunnel.policyRef(), Err: err}
		}
		return sdk.BaseClusterResources{Listeners: []*envoylistenerv3.Listener{listener}}, nil
	}
}

// translateTunnelResources validates the tunnel, rewrites the destination
// cluster, and returns its CONNECT listener.
func translateTunnelResources(
	kctx krt.HandlerContext,
	backends *krtcollections.BackendIndex,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
	tunnel *tunnelIR,
) (*envoylistenerv3.Listener, error) {
	target, err := tunnelDestination(in)
	if err != nil {
		return nil, err
	}
	if err := validateTunnelCluster(out); err != nil {
		return nil, err
	}
	proxyCluster, err := resolveTunnelProxy(kctx, backends, tunnel)
	if err != nil {
		return nil, err
	}
	listener, err := buildTunnelListener(out.GetName(), proxyCluster, target, tunnel.headers)
	if err != nil {
		return nil, err
	}
	rewriteTunnelCluster(out, target)
	return listener, nil
}

// effectiveTunnel returns the tunnel selected by policy precedence.
// Errored policies fail base translation before resource hooks run.
func effectiveTunnel(in ir.BackendObjectIR) *tunnelIR {
	policies := in.AttachedPolicies.Policies[wellknown.BackendConfigPolicyGVK.GroupKind()]
	if !slices.ContainsFunc(policies, setsTunnel) {
		return nil
	}
	merged, ok := mergePolicies(policies).PolicyIr.(*BackendConfigPolicyIR)
	if !ok {
		return nil
	}
	return merged.tunnel
}

func setsTunnel(att ir.PolicyAtt) bool {
	pol, ok := att.PolicyIr.(*BackendConfigPolicyIR)
	return ok && pol.tunnel != nil
}

// tunnelDestination returns the host and port of a single-host Static Backend.
func tunnelDestination(in ir.BackendObjectIR) (tunnelTarget, error) {
	backend, ok := in.Obj.(*kgateway.Backend)
	if !ok || backend.Spec.Static == nil {
		return tunnelTarget{}, errors.New("tunnel: only Static Backends can be tunneled")
	}
	hosts := backend.Spec.Static.Hosts
	if len(hosts) != 1 {
		return tunnelTarget{}, fmt.Errorf("tunnel: a tunneled Static Backend must have exactly one host, found %d", len(hosts))
	}
	return tunnelTarget{
		host: hosts[0].Host,
		port: uint32(hosts[0].Port), //nolint:gosec // G115: port is validated as 1-65535
	}, nil
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

// tunnelName hashes the cluster name to fit the Unix socket path limit.
func tunnelName(clusterName string) string {
	return fmt.Sprintf("connect_tunnel_%016x", utils.HashString(clusterName))
}

// tunnelSocketAddress uses Envoy's @ prefix for Linux abstract Unix sockets.
func tunnelSocketAddress(clusterName string) *envoycorev3.Address {
	return &envoycorev3.Address{
		Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: "@" + tunnelName(clusterName)}},
	}
}

// rewriteTunnelCluster replaces discovery with a pipe endpoint, preserving
// destination TLS and the hostname used by autoHostRewrite.
func rewriteTunnelCluster(out *envoyclusterv3.Cluster, target tunnelTarget) {
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}
	out.EdsClusterConfig = nil
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
			LbEndpoints: []*envoyendpointv3.LbEndpoint{{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Hostname: target.host,
						Address:  tunnelSocketAddress(out.GetName()),
					},
				},
			}},
		}},
	}
}

// buildTunnelListener creates a Unix listener that CONNECTs through the proxy cluster.
func buildTunnelListener(
	clusterName, proxyCluster string,
	target tunnelTarget,
	headers []*envoycorev3.HeaderValueOption,
) (*envoylistenerv3.Listener, error) {
	name := tunnelName(clusterName)
	tcpProxy := &envoytcp.TcpProxy{
		StatPrefix:       name,
		ClusterSpecifier: &envoytcp.TcpProxy_Cluster{Cluster: proxyCluster},
		TunnelingConfig: &envoytcp.TcpProxy_TunnelingConfig{
			Hostname:     strings.ReplaceAll(target.authority(), "%", "%%"),
			HeadersToAdd: headers,
		},
	}
	config, err := utils.MessageToAny(tcpProxy)
	if err != nil {
		return nil, fmt.Errorf("tunnel listener: %w", err)
	}
	return &envoylistenerv3.Listener{
		Name:       name,
		StatPrefix: name,
		Address:    tunnelSocketAddress(clusterName),
		FilterChains: []*envoylistenerv3.FilterChain{{
			Filters: []*envoylistenerv3.Filter{{
				Name:       envoywellknown.TCPProxy,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: config},
			}},
		}},
	}, nil
}

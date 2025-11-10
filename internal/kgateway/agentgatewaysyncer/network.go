package agentgatewaysyncer

import (
	"net/netip"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/api/annotation"
	"istio.io/api/label"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/kind"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/network"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// NetworkGateway represents a gateway that provides connectivity to a specific network
type NetworkGateway struct {
	// Network is the ID of the network this gateway provides access to
	Network network.ID
	// Cluster is the ID of the k8s cluster where this Gateway resides
	Cluster cluster.ID
	// Addr is the gateway address (IP or hostname)
	Addr string
	// Port is the gateway port
	Port uint32
	// HBONEPort indicates that the gateway supports HBONE on this port
	HBONEPort uint32
	// ServiceAccount the gateway runs as
	ServiceAccount types.NamespacedName
	// Source is the Gateway resource that this NetworkGateway was derived from
	Source types.NamespacedName
}

func (n NetworkGateway) ResourceName() string {
	return n.Source.String() + "/" + n.Addr
}

func (n NetworkGateway) Equals(other NetworkGateway) bool {
	return n.Network == other.Network &&
		n.Cluster == other.Cluster &&
		n.Addr == other.Addr &&
		n.Port == other.Port &&
		n.HBONEPort == other.HBONEPort &&
		n.ServiceAccount == other.ServiceAccount &&
		n.Source == other.Source
}

// NetworkGatewaysCollection builds a collection of NetworkGateway objects from Gateway resources
// that have the topology.istio.io/network label set.
func (a *index) NetworkGatewaysCollection(
	gateways krt.Collection[*gatewayv1.Gateway],
	krtopts krtutil.KrtOptions,
) (krt.Collection[NetworkGateway], krt.Index[network.ID, NetworkGateway]) {
	networkGateways := krt.NewManyCollection(
		gateways,
		func(ctx krt.HandlerContext, gw *gatewayv1.Gateway) []NetworkGateway {
			return k8sGatewayToNetworkGateways(cluster.ID(a.ClusterID), gw)
		},
		krtopts.ToOptions("NetworkGateways")...,
	)

	gatewaysByNetwork := krt.NewIndex(networkGateways, "network", func(o NetworkGateway) []network.ID {
		return []network.ID{o.Network}
	})

	return networkGateways, gatewaysByNetwork
}

// k8sGatewayToNetworkGateways converts a Gateway resource to NetworkGateway objects.
// It looks for Gateways with:
// 1. topology.istio.io/network label set
// 2. GatewayClassName of "istio-remote" (for declaring gateways that provide access to other networks)
// 3. At least one HBONE listener
func k8sGatewayToNetworkGateways(clusterID cluster.ID, gw *gatewayv1.Gateway) []NetworkGateway {
	// Check if this gateway has a network label
	netLabel := gw.GetLabels()[label.TopologyNetwork.Name]
	if netLabel == "" {
		return nil
	}

	// Only process gateways with istio-remote gateway class
	// These are used to declare gateways that provide access to other networks
	if gw.Spec.GatewayClassName != constants.RemoteGatewayClassName {
		return nil
	}

	// No addresses means the gateway isn't ready yet
	if len(gw.Status.Addresses) == 0 {
		return nil
	}

	base := NetworkGateway{
		Network: network.ID(netLabel),
		Cluster: clusterID,
		ServiceAccount: types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      getGatewaySA(gw),
		},
		Source: config.NamespacedName(gw),
	}

	var gateways []NetworkGateway

	// Process each address in the gateway
	for _, addr := range gw.Status.Addresses {
		if addr.Type == nil {
			continue
		}
		addrType := *addr.Type
		if addrType != gatewayv1.IPAddressType && addrType != gatewayv1.HostnameAddressType {
			continue
		}

		// Look for HBONE listeners
		for _, l := range gw.Spec.Listeners {
			if l.Protocol == "HBONE" {
				networkGateway := base
				networkGateway.Addr = addr.Value
				networkGateway.Port = uint32(l.Port)
				networkGateway.HBONEPort = uint32(l.Port)
				gateways = append(gateways, networkGateway)
				break // Only need one HBONE listener per address
			}
		}
	}

	return gateways
}

// getGatewaySA returns the service account for a gateway.
// If the gateway has a service account annotation, use that.
// Otherwise, default to the gateway name with "-istio" suffix.
func getGatewaySA(gw *gatewayv1.Gateway) string {
	if sa, ok := gw.Annotations[annotation.GatewayServiceAccount.Name]; ok {
		return sa
	}
	// Default service account name for Istio gateways
	return gw.Name + "-istio"
}

// LookupNetworkGateway finds network gateways for the given network
func LookupNetworkGateway(
	ctx krt.HandlerContext,
	nw network.ID,
	networkGateways krt.Collection[NetworkGateway],
	gatewaysByNetwork krt.Index[network.ID, NetworkGateway],
) []NetworkGateway {
	if nw == "" {
		// Default network, no gateway needed
		return nil
	}
	return krt.Fetch(ctx, networkGateways, krt.FilterIndex(gatewaysByNetwork, nw))
}

// networkGatewayToWorkload converts a NetworkGateway to a WorkloadInfo
// This creates gateway workloads that agentgateway uses to establish mTLS connections
func (a *index) networkGatewayToWorkload(ctx krt.HandlerContext, ng NetworkGateway) *WorkloadInfo {
	// Parse the gateway address
	addr, err := netip.ParseAddr(ng.Addr)
	if err != nil {
		// If not an IP, treat as hostname
		return nil
	}

	w := &api.Workload{
		Uid:               a.ClusterID + "/gateway/" + ng.Source.Namespace + "/" + ng.Source.Name + "-" + string(ng.Network),
		Name:              ng.Source.Name + "-" + string(ng.Network),
		Namespace:         ng.Source.Namespace,
		ClusterId:         a.ClusterID,
		Addresses:         [][]byte{addr.AsSlice()},
		Network:           "", // Gateway is on the local network
		ServiceAccount:    ng.ServiceAccount.Name,
		TunnelProtocol:    api.TunnelProtocol_HBONE,
		TrustDomain:       pickTrustDomain(),
		Status:            api.WorkloadStatus_HEALTHY,
		WorkloadName:      ng.Source.Name,
		WorkloadType:      api.WorkloadType_POD,
		CanonicalName:     ng.Source.Name,
		CanonicalRevision: "latest",
		Services:          map[string]*api.PortList{},
	}

	return precomputeWorkloadPtr(&WorkloadInfo{
		Workload: w,
		Labels:   map[string]string{"app": ng.Source.Name},
		Source:   kind.Gateway,
	})
}

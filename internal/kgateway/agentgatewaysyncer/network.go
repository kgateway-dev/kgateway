package agentgatewaysyncer

import (
	"net/netip"
	"strconv"

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
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// NetworkGatewaysCollection builds a collection of NetworkGateway objects from Gateway resources
// that have the topology.istio.io/network label set.
func (a *index) NetworkGatewaysCollection(
	gateways krt.Collection[*gwv1.Gateway],
	krtopts krtutil.KrtOptions,
) (krt.Collection[translator.NetworkGateway], krt.Index[network.ID, translator.NetworkGateway]) {
	networkGateways := krt.NewManyCollection(
		gateways,
		func(ctx krt.HandlerContext, gw *gwv1.Gateway) []translator.NetworkGateway {
			return k8sGatewayToNetworkGateways(cluster.ID(a.ClusterID), gw)
		},
		krtopts.ToOptions("NetworkGateways")...,
	)

	gatewaysByNetwork := krt.NewIndex(networkGateways, "network", func(o translator.NetworkGateway) []network.ID {
		return []network.ID{o.Network}
	})

	return networkGateways, gatewaysByNetwork
}

// k8sGatewayToNetworkGateways converts a Gateway resource to NetworkGateway objects.
// It looks for Gateways with:
// 1. topology.istio.io/network label set
// 2. GatewayClassName of "istio-remote" (for declaring gateways that provide access to other networks)
// 3. At least one HBONE listener
func k8sGatewayToNetworkGateways(clusterID cluster.ID, gw *gwv1.Gateway) []translator.NetworkGateway {
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

	base := translator.NetworkGateway{
		Network: network.ID(netLabel),
		Cluster: clusterID,
		ServiceAccount: types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      getGatewaySA(gw),
		},
		Source: config.NamespacedName(gw),
	}

	var gateways []translator.NetworkGateway

	// Process each address in the gateway
	for _, addr := range gw.Status.Addresses {
		if addr.Type == nil {
			continue
		}
		addrType := *addr.Type
		if addrType != gwv1.IPAddressType && addrType != gwv1.HostnameAddressType {
			continue
		}

		// Look for HBONE listeners
		for _, l := range gw.Spec.Listeners {
			if l.Protocol == "HBONE" {
				networkGateway := base
				networkGateway.Addr = addr.Value
				networkGateway.Port = uint32(l.Port)      //nolint:gosec // G115: Gateway listener port is always in valid range
				networkGateway.HBONEPort = uint32(l.Port) //nolint:gosec // G115: Gateway listener port is always in valid range
				gateways = append(gateways, networkGateway)
				break // Only need one HBONE listener per address
			}
		}
	}

	return gateways
}

// getGatewaySA returns the service account for a gateway.
// If the gateway has a service account annotation, use that.
// Otherwise, use gateway name with appropriate suffix based on GatewayClassName to match Istio conventions.
func getGatewaySA(gw *gwv1.Gateway) string {
	if sa, ok := gw.Annotations[annotation.GatewayServiceAccount.Name]; ok {
		return sa
	}
	// Use appropriate suffix based on GatewayClassName to match Istio conventions
	if gw.Spec.GatewayClassName == constants.RemoteGatewayClassName {
		return gw.Name + "-" + constants.RemoteGatewayClassName
	}
	// Default service account name for other gateways
	return gw.Name
}

// networkGatewayToWorkload converts a NetworkGateway to a WorkloadInfo
// This creates gateway workloads that agentgateway uses to establish mTLS connections
func (a *index) networkGatewayToWorkload(ctx krt.HandlerContext, ng translator.NetworkGateway) *WorkloadInfo {
	w := &api.Workload{
		Uid:               a.ClusterID + "/gateway/" + string(ng.Network) + "/" + ng.Addr + "/" + strconv.Itoa(int(ng.HBONEPort)),
		Name:              ng.Source.Name + "-" + string(ng.Network) + "-" + ng.Addr,
		Namespace:         ng.Source.Namespace,
		ClusterId:         a.ClusterID,
		Network:           string(ng.Network), // Gateway provides access to this network
		ServiceAccount:    ng.ServiceAccount.Name,
		TunnelProtocol:    api.TunnelProtocol_HBONE,
		TrustDomain:       pickTrustDomain(),
		Status:            api.WorkloadStatus_HEALTHY,
		WorkloadName:      ng.Source.Name,
		CanonicalName:     ng.Source.Name,
		CanonicalRevision: "latest",
		Services:          map[string]*api.PortList{},
	}

	// Parse the gateway address
	if addr, err := netip.ParseAddr(ng.Addr); err == nil {
		w.Addresses = [][]byte{addr.AsSlice()}
		w.Hostname = ""
	} else {
		// If not an IP, treat as hostname
		w.Hostname = ng.Addr
		w.Addresses = nil
	}

	return precomputeWorkloadPtr(&WorkloadInfo{
		Workload: w,
		Labels:   map[string]string{"app": ng.Source.Name},
		Source:   kind.Gateway,
	})
}

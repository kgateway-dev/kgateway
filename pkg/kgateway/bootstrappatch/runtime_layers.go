package bootstrappatch

import (
	"fmt"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"k8s.io/apimachinery/pkg/types"
)

// xdsClusterName is the generated static cluster that connects the proxy to the
// controller's xDS server.
const xdsClusterName = "xds_cluster"

// Kinds of runtime layer whose values the proxy loads at runtime.
const (
	RuntimeLayerDisk = "disk"
	RuntimeLayerRTDS = "RTDS"
)

// RuntimeLayer is a disk or RTDS runtime layer an overlay adds. The bootstrap says only
// where its values come from, so they are not in the validation subset, and route
// validation uses Envoy's defaults for them.
type RuntimeLayer struct {
	// Name is the layer's name.
	Name string
	// Kind is RuntimeLayerDisk or RuntimeLayerRTDS.
	Kind string
	// Source is the GatewayParameters whose overlay adds the layer.
	Source types.NamespacedName
}

func (l RuntimeLayer) String() string {
	return fmt.Sprintf("%s layer %q (GatewayParameters %s)", l.Kind, l.Name, l.Source)
}

// unvalidatedRuntimeLayers returns the overlay's disk and RTDS runtime layers, in order.
func unvalidatedRuntimeLayers(ov *envoybootstrapv3.Bootstrap, source types.NamespacedName) []RuntimeLayer {
	var out []RuntimeLayer
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		switch {
		case l.GetDiskLayer() != nil:
			out = append(out, RuntimeLayer{Name: l.GetName(), Kind: RuntimeLayerDisk, Source: source})
		case l.GetRtdsLayer() != nil:
			out = append(out, RuntimeLayer{Name: l.GetName(), Kind: RuntimeLayerRTDS, Source: source})
		}
	}
	return out
}

// checkRTDSLayers rejects an RTDS layer that requests its values from kgateway's xDS
// server, through ADS or the xds_cluster. kgateway serves no runtime resources, so the
// layer would never receive values.
func checkRTDSLayers(ov *envoybootstrapv3.Bootstrap) error {
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		// Only RTDS layers request values from a server.
		rtds := l.GetRtdsLayer()
		if rtds == nil {
			continue
		}

		// Reject a layer that uses ADS or names the xds_cluster.
		cs := rtds.GetRtdsConfig()
		via := ""
		switch {
		case cs.GetAds() != nil:
			via = "ADS"
		case usesCluster(cs.GetApiConfigSource(), xdsClusterName):
			via = "the " + xdsClusterName + " cluster"
		}
		if via != "" {
			return fmt.Errorf("RTDS runtime layer %q requests its values from the kgateway control plane through %s, which serves no runtime values; point rtds_config at another server, or use a static or disk layer", l.GetName(), via)
		}
	}
	return nil
}

// usesCluster reports whether an API config source sends requests to the named cluster,
// either by cluster name or through an Envoy gRPC service.
func usesCluster(api *envoycorev3.ApiConfigSource, cluster string) bool {
	// A REST or delta config source lists its clusters by name.
	if slices.Contains(api.GetClusterNames(), cluster) {
		return true
	}

	// A gRPC config source names the cluster of each Envoy gRPC service.
	for _, svc := range api.GetGrpcServices() {
		if svc.GetEnvoyGrpc().GetClusterName() == cluster {
			return true
		}
	}
	return false
}

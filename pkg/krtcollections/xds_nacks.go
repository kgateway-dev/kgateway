package krtcollections

import (
	"strings"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

// ADS acknowledgment is per type URL: Envoy can accept the CDS of a snapshot
// version while rejecting its RDS, leaving an applied combination that was
// never one of our published snapshots, and a persistently-rejected type
// stays frozen at its last-accepted config while every other type advances.
// Nothing in go-control-plane surfaces this; the only place a rejection is
// visible to the control plane is the DiscoveryRequest that carries it. This
// counter makes that visible: a healthy installation is always at zero, any
// increment means Envoy rejected a kgateway-generated config, and a sustained
// rate on one type URL is the resend/reject loop of a translator bug.
var xdsNacksTotal = metrics.NewCounter(
	metrics.CounterOpts{
		Subsystem: "xds",
		Name:      "nacks_total",
		Help: "Total xDS responses NACKed by connected clients, by type URL. " +
			"Envoy rejects a response in toto for that type and keeps serving " +
			"its last-accepted config, so a nonzero value means some client is " +
			"running older config than the control plane published for that " +
			"type — and possibly a mixed application of two snapshot versions " +
			"across types. Any increment is a kgateway translation bug worth " +
			"reporting with the accompanying warning log line.",
	},
	[]string{"type_url", "gateway", "namespace"},
)

// recordNackIfAny inspects a DiscoveryRequest and, when it carries an
// ErrorDetail (the xDS NACK signal), logs the rejection and counts it. Called
// after initialization, peer resolution, and successful stream processing. The
// role must come from authenticated identity, or be empty when auth is disabled.
func recordNackIfAny(role string, r *envoy_service_discovery_v3.DiscoveryRequest) {
	errDetail := r.GetErrorDetail()
	if errDetail == nil {
		return
	}
	typeURL := shortTypeURL(r.GetTypeUrl())
	gateway, namespace := gatewayFromRole(role)
	logger.Warn("client NACKed xDS response; it stays on its last-accepted config for this type",
		"type_url", typeURL,
		"gateway", gateway,
		"namespace", namespace,
		"last_accepted_version", r.GetVersionInfo(),
		"nonce", r.GetResponseNonce(),
		"error", errDetail.GetMessage(),
	)
	if !metrics.Active() {
		return
	}
	xdsNacksTotal.Inc(
		metrics.Label{Name: "type_url", Value: typeURL},
		metrics.Label{Name: "gateway", Value: gateway},
		metrics.Label{Name: "namespace", Value: namespace},
	)
}

// shortTypeURL maps known resource URLs to short names and all other input
// to a fixed label, bounding cardinality even for arbitrary client input.
func shortTypeURL(typeURL string) string {
	switch typeURL {
	case resourcev3.ClusterType:
		return "Cluster"
	case resourcev3.EndpointType:
		return "ClusterLoadAssignment"
	case resourcev3.RouteType:
		return "RouteConfiguration"
	case resourcev3.ScopedRouteType:
		return "ScopedRouteConfiguration"
	case resourcev3.VirtualHostType:
		return "VirtualHost"
	case resourcev3.ListenerType:
		return "Listener"
	case resourcev3.SecretType:
		return "Secret"
	case resourcev3.ExtensionConfigType:
		return "TypedExtensionConfig"
	case resourcev3.RuntimeType:
		return "Runtime"
	default:
		return "other"
	}
}

// gatewayFromRole extracts the gateway name and namespace from a kgateway
// proxy role ("kgateway-kube-gateway-api~<ns>~<name>") or the unique cache
// key derived from it ("...~<ns>~<name>~<labels-hash>~<pod-ns>" keeps the
// same first three segments).
func gatewayFromRole(role string) (gateway, namespace string) {
	parts := strings.Split(role, "~")
	if len(parts) >= 3 {
		return parts[2], parts[1]
	}
	return "unknown", "unknown"
}

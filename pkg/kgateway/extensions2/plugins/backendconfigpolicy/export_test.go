package backendconfigpolicy

import (
	"time"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// NewTLSPolicyIRForTest returns a BackendConfigPolicy IR that sets only spec.tls.
func NewTLSPolicyIRForTest(ct time.Time, tls *envoytlsv3.UpstreamTlsContext) ir.PolicyIR {
	return &BackendConfigPolicyIR{ct: ct, tlsConfig: tls}
}

// ProcessBackendForTest is the plugin's ProcessBackend hook.
var ProcessBackendForTest = processBackend

// MergePoliciesForTest merges BackendConfigPolicies as translation does.
var MergePoliciesForTest = mergePolicies

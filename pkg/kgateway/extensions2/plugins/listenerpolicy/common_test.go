package listenerpolicy

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendconfigpolicy"
	kwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestToEnvoyHttp(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "Backend",
		Name:      "test-service",
		Namespace: "default",
	}, 4317, "", "")
	backend.CanonicalHostname = "otel-collector.default.svc.cluster.local"

	customPath := "/custom/otlp/logs"
	testCases := []struct {
		name     string
		in       kgateway.CommonHttpService
		expected string
	}{
		{
			name:     "defaults to the signal's OTLP path when unset",
			in:       kgateway.CommonHttpService{},
			expected: "http://otel-collector.default.svc.cluster.local:4317/v1/logs",
		},
		{
			name:     "uses the user-provided path when set",
			in:       kgateway.CommonHttpService{Path: &customPath},
			expected: "http://otel-collector.default.svc.cluster.local:4317/custom/otlp/logs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpService, err := ToEnvoyHttp(tc.in, &backend, OTLPHTTPLogsPath)
			require.NoError(t, err)

			// Uri is built from the backend's own hostname/port plus the path, since
			// CommonHttpService has no literal URL of its own. Cluster is resolved independently.
			assert.Equal(t, tc.expected, httpService.GetHttpUri().GetUri())
			assert.Equal(t, "backend_default_test-service_4317", httpService.GetHttpUri().GetCluster())
			assert.IsType(t, &envoycorev3.HttpUri_Cluster{}, httpService.GetHttpUri().GetHttpUpstreamType())
		})
	}
}

func TestToEnvoyHttp_WithBackendTLSPolicy(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "Backend",
		Name:      "otel-collector",
		Namespace: "default",
	}, 4318, "", "")
	backend.CanonicalHostname = "otel.example.com"
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{
			kwellknown.BackendTLSPolicyGVK.GroupKind(): {{}},
		},
	}

	httpService, err := ToEnvoyHttp(kgateway.CommonHttpService{}, &backend, OTLPHTTPTracesPath)
	require.NoError(t, err)

	// A BackendTLSPolicy attached to the backend means the scheme should read https, even
	// though TLS itself is applied to the cluster separately (not by ToEnvoyHttp).
	assert.Equal(t, "https://otel.example.com:4318/v1/traces", httpService.GetHttpUri().GetUri())
}

func TestToEnvoyHttp_WithBackendConfigPolicy(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "Backend",
		Name:      "otel-collector",
		Namespace: "default",
	}, 4318, "", "")
	backend.CanonicalHostname = "otel.example.com"
	backend.AttachedPolicies = ir.AttachedPolicies{
		Policies: map[schema.GroupKind][]ir.PolicyAtt{
			kwellknown.BackendConfigPolicyGVK.GroupKind(): {
				{PolicyIr: &backendconfigpolicy.BackendConfigPolicyIR{}},
			},
		},
	}

	httpService, err := ToEnvoyHttp(kgateway.CommonHttpService{}, &backend, OTLPHTTPTracesPath)
	require.NoError(t, err)

	// A BackendConfigPolicy attached without TLS configured shouldn't flip the scheme - mere
	// attachment isn't a reliable signal, unlike BackendTLSPolicy.
	assert.Equal(t, "http://otel.example.com:4318/v1/traces", httpService.GetHttpUri().GetUri())
}

func TestToEnvoyHttp_NoResolvableHostname(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Kind:      "Backend",
		Name:      "lambda-backend",
		Namespace: "default",
	}, 0, "", "")
	// CanonicalHostname left unset, as is the case for backend kinds with no inherent
	// network address (e.g. AWS Lambda, DynamicForwardProxy).

	_, err := ToEnvoyHttp(kgateway.CommonHttpService{}, &backend, OTLPHTTPLogsPath)
	assert.Error(t, err)
}

package irtranslator_test

import (
	"context"
	"errors"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

func TestBackendTranslatorTranslatesAppProtocol(t *testing.T) {
	var bt irtranslator.BackendTranslator
	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		AppProtocol: ir.HTTP2AppProtocol,
	}
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}

	c, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	require.NoError(t, err)
	opts := c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	assert.NotNil(t, opts)

	p, err := opts.UnmarshalNew()
	require.NoError(t, err)

	httpOpts, ok := p.(*envoy_upstreams_v3.HttpProtocolOptions)
	assert.True(t, ok)
	assert.NotNil(t, httpOpts.GetExplicitHttpConfig().GetHttp2ProtocolOptions())
}

// TestBackendTranslatorHandlesBackendIRErrors validates that when the Backend IR itself
// has pre-existing errors, the translator returns a blackhole cluster and error.
func TestBackendTranslatorHandlesBackendIRErrors(t *testing.T) {
	// Create backend IR errors to simulate validation failures during IR construction.
	// No attached policies needed for this test.
	backendError1 := errors.New("invalid backend hostname")
	backendError2 := errors.New("unsupported backend protocol")
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "core",
			Kind:      "Service",
			Name:      "invalid-svc",
			Namespace: "test-ns",
		},
		Port:   80,
		Errors: []error{backendError1, backendError2},
		AttachedPolicies: pluginsdkir.AttachedPolicies{
			Policies: map[schema.GroupKind][]pluginsdkir.PolicyAtt{},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	// Validate that the backend IR errors are propagated.
	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid backend hostname")
	assert.Contains(t, err.Error(), "unsupported backend protocol")

	// Should return a blackhole cluster when Backend IR has errors
	assert.NotNil(t, cluster)
	assert.Equal(t, "service_test-ns_invalid-svc_80", cluster.GetName())
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType())
	assert.Empty(t, cluster.GetLoadAssignment().GetEndpoints())

	// Backend IR errors should remain in the backend
	assert.NotEmpty(t, backend.Errors)
	assert.Contains(t, backend.Errors, backendError1)
	assert.Contains(t, backend.Errors, backendError2)
}

// TestBackendTranslatorPropagatesPolicyErrors validates that attached policy IR errors
// are propagated and result in an error return with a blackhole cluster.
func TestBackendTranslatorPropagatesPolicyErrors(t *testing.T) {
	policyError1 := errors.New("invalid TLS certificate")
	policyError2 := errors.New("invalid health check configuration")
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "group",
			Kind:      "kind",
			Name:      "name",
			Namespace: "namespace",
		},
		AttachedPolicies: pluginsdkir.AttachedPolicies{
			Policies: map[schema.GroupKind][]pluginsdkir.PolicyAtt{
				{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {
					{
						GroupKind: schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"},
						Errors:    []error{policyError1},
					},
				},
				{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
					{
						GroupKind: schema.GroupKind{Group: "gateway-api", Kind: "BackendTLSPolicy"},
						Errors:    []error{policyError2},
					},
				},
			},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "group", Kind: "kind"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {
			Name: "BackendConfigPolicy",
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
			},
		},
		{Group: "gateway-api", Kind: "BackendTLSPolicy"}: {
			Name: "BackendTLSPolicy",
			ProcessBackend: func(ctx context.Context, polir ir.PolicyIR, backend ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
			},
		},
	}

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)
	// Validate that the policy errors are propagated.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid TLS certificate")
	assert.Contains(t, err.Error(), "invalid health check configuration")

	// Validate that a blackhole cluster is returned when policy errors occur.
	assert.NotNil(t, cluster)
	assert.Equal(t, "kind_namespace_name_0", cluster.GetName())
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType())
	assert.Empty(t, cluster.GetLoadAssignment().GetEndpoints())

	// Validate that policy errors are not stored in backend.Errors
	assert.Empty(t, backend.Errors)
}

// TestBackendTranslatorHandlesXDSValidationErrors validates that when xDS validation fails
// in strict mode, the translator returns a blackhole cluster and error.
func TestBackendTranslatorHandlesXDSValidationErrors(t *testing.T) {
	// Create a mock validator that always returns an error
	mockValidator := &mockValidator{
		validateFunc: func(ctx context.Context, config string) error {
			return errors.New("envoy validation failed: invalid cluster configuration")
		},
	}

	// BackendIR with no errors.
	backend := &ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "core",
			Kind:      "Service",
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Port: 80,
		// No pre-existing errors
		Errors: nil,
		// No attached policies that would cause errors
		AttachedPolicies: pluginsdkir.AttachedPolicies{
			Policies: map[schema.GroupKind][]pluginsdkir.PolicyAtt{},
		},
	}

	var bt irtranslator.BackendTranslator
	bt.ContributedBackends = map[schema.GroupKind]ir.BackendInit{
		{Group: "core", Kind: "Service"}: {
			InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
				return nil
			},
		},
	}
	bt.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{}

	// Set up strict mode and inject the mock validator
	bt.Mode = apisettings.ValidationStrict
	bt.Validator = mockValidator

	var ucc ir.UniqlyConnectedClient
	var kctx krt.TestingDummyContext
	cluster, err := bt.TranslateBackend(context.Background(), kctx, ucc, backend)

	// Should get an error because xDS validation failed
	require.Error(t, err)
	assert.Contains(t, err.Error(), "envoy validation failed")
	assert.Contains(t, err.Error(), "invalid cluster configuration")

	// Should return a blackhole cluster when xDS validation fails
	assert.NotNil(t, cluster)
	assert.Equal(t, "service_test-ns_test-svc_80", cluster.GetName())
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, cluster.GetType())
	assert.Empty(t, cluster.GetLoadAssignment().GetEndpoints())

	// Backend IR should remain clean (xDS errors don't modify backend.Errors)
	assert.Empty(t, backend.Errors)
}

// mockValidator is a test implementation of validator.Validator for testing xDS validation errors
type mockValidator struct {
	validateFunc func(ctx context.Context, config string) error
}

var _ validator.Validator = &mockValidator{}

func (m *mockValidator) Validate(ctx context.Context, config string) error {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, config)
	}
	return nil
}

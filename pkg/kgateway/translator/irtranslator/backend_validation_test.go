package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

type recordingValidator struct {
	calls atomic.Int64
	err   error
}

func (v *recordingValidator) Validate(_ context.Context, _ *envoybootstrapv3.Bootstrap) error {
	v.calls.Add(1)
	return v.err
}

func validationTestTranslator(v validator.Validator) *BackendTranslator {
	return &BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		Validator: v,
		Mode:      apisettings.ValidationStrict,
	}
}

func validationTestBackend(name string) *ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 443, "", "")
	return &b
}

func translateValidationTestBase(t *testing.T, tr *BackendTranslator, backend *ir.BackendObjectIR) (*envoyclusterv3.Cluster, error) {
	t.Helper()
	base := tr.TranslateBackendBase(krt.TestingDummyContext{}, t.Context(), backend)
	require.NotNil(t, base)
	return base.Cluster, base.Error
}

func TestStrictValidationCachedByClusterContent(t *testing.T) {
	counting := &recordingValidator{}
	tr := validationTestTranslator(validator.NewCaching(counting, 0))

	for range 5 {
		c, err := translateValidationTestBase(t, tr, validationTestBackend("b1"))
		require.NoError(t, err)
		require.NotNil(t, c)
	}
	require.EqualValues(t, 1, counting.calls.Load(), "identical cluster content must be validated once")

	_, err := translateValidationTestBase(t, tr, validationTestBackend("b2"))
	require.NoError(t, err)
	require.EqualValues(t, 2, counting.calls.Load(), "distinct cluster content must be validated separately")
}

func TestStrictValidationCachesInvalidVerdicts(t *testing.T) {
	counting := &recordingValidator{err: fmt.Errorf("%w: bad cluster", validator.ErrInvalidXDS)}
	tr := validationTestTranslator(validator.NewCaching(counting, 0))

	for range 3 {
		c, err := translateValidationTestBase(t, tr, validationTestBackend("b1"))
		require.ErrorIs(t, err, validator.ErrInvalidXDS)
		require.NotNil(t, c, "errored translation returns the blackhole cluster")
	}
	require.EqualValues(t, 1, counting.calls.Load(), "an invalid verdict must be cached by content")
}

func TestStrictValidationDoesNotCacheTransientErrors(t *testing.T) {
	counting := &recordingValidator{err: errors.New("exec: envoy binary not found")}
	tr := validationTestTranslator(validator.NewCaching(counting, 0))

	_, err := translateValidationTestBase(t, tr, validationTestBackend("b1"))
	require.Error(t, err)

	counting.err = nil
	c, err := translateValidationTestBase(t, tr, validationTestBackend("b1"))
	require.NoError(t, err)
	require.NotNil(t, c)
	require.EqualValues(t, 2, counting.calls.Load(), "a transient failure must not be served from cache")
}

// memoTestTranslator is validationTestTranslator plus an overlay that mutates the
// cluster for every client, identically unless the client carries a "variant"
// label, so per-client validation runs for every (client, backend) pair and
// the memo has byte-identical clusters to collapse.
func memoTestTranslator(v validator.Validator, memo *validator.Memo) *BackendTranslator {
	tr := validationTestTranslator(v)
	tr.ValidationMemo = memo
	tr.ContributedPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
		{Group: "test", Kind: "Overlay"}: {
			PerClientClusterOverlay: func(_ krt.HandlerContext, _ context.Context, ucc ir.UniquelyConnectedClient, _ ir.BackendObjectIR) *sdk.ClusterOverlay {
				variant := ucc.Labels["variant"]
				return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
					out.AltStatName = "overlaid" + variant
				}}
			},
		},
	}
	return tr
}

func memoTestClient(name string, labels map[string]string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(name, "ns", labels, ir.PodLocality{})
}

// Per-client validation runs once per connected client on every walk over the
// backends. With the memo, clients whose overlaid cluster comes out byte-identical
// share one verdict, and a repeat walk validates nothing.
func TestStrictValidationMemoCollapsesIdenticalPerClientClusters(t *testing.T) {
	counting := &recordingValidator{}
	tr := memoTestTranslator(counting, validator.NewMemo(0))
	backend := validationTestBackend("b1")

	base := tr.TranslateBackendBase(krt.TestingDummyContext{}, t.Context(), backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	require.EqualValues(t, 1, counting.calls.Load(), "the base is validated once")

	clients := []ir.UniquelyConnectedClient{
		memoTestClient("a", nil), memoTestClient("b", nil), memoTestClient("c", nil),
	}
	for walk := range 3 {
		for _, ucc := range clients {
			perClient, err := tr.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), ucc, backend, base)
			require.NoError(t, err)
			require.NotNil(t, perClient, "the overlay applies to every client")
		}
		require.EqualValues(t, 2, counting.calls.Load(),
			"walk %d: identical per-client clusters must be validated once across clients and walks", walk)
	}

	_, err := tr.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), memoTestClient("d", map[string]string{"variant": "-d"}), backend, base)
	require.NoError(t, err)
	require.EqualValues(t, 3, counting.calls.Load(), "a client whose cluster differs must be validated")
}

// Without a memo the translator validates every pair, which is what the memo
// is measured against and what tests built from a bare BackendTranslator get.
func TestStrictValidationWithoutMemoValidatesEveryPair(t *testing.T) {
	counting := &recordingValidator{}
	tr := memoTestTranslator(counting, nil)
	backend := validationTestBackend("b1")

	base := tr.TranslateBackendBase(krt.TestingDummyContext{}, t.Context(), backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	for _, name := range []string{"a", "b", "c"} {
		_, err := tr.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), memoTestClient(name, nil), backend, base)
		require.NoError(t, err)
	}
	require.EqualValues(t, 4, counting.calls.Load(), "base once plus one per client")
}

// A memoized invalid verdict must surface for every client with its identity and
// message intact, so status reports the same error the validator produced.
func TestStrictValidationMemoKeepsInvalidVerdictIdentity(t *testing.T) {
	counting := &recordingValidator{err: fmt.Errorf("%w: bad overlaid cluster", validator.ErrInvalidXDS)}
	tr := memoTestTranslator(counting, validator.NewMemo(0))
	backend := validationTestBackend("b1")
	// Let the base pass so the per-client path is what fails.
	counting.err = nil
	base := tr.TranslateBackendBase(krt.TestingDummyContext{}, t.Context(), backend)
	require.NotNil(t, base)
	require.NoError(t, base.Error)
	counting.err = fmt.Errorf("%w: bad overlaid cluster", validator.ErrInvalidXDS)

	var errs []error
	for _, name := range []string{"a", "b"} {
		_, err := tr.ApplyPerClient(krt.TestingDummyContext{}, t.Context(), memoTestClient(name, nil), backend, base)
		require.ErrorIs(t, err, validator.ErrInvalidXDS)
		errs = append(errs, err)
	}
	require.Equal(t, errs[0].Error(), errs[1].Error(), "the memoized message must match the original")
	require.EqualValues(t, 2, counting.calls.Load(), "base once, the shared invalid verdict once")
}

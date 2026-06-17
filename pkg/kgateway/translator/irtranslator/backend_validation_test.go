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

func TestStrictValidationCachedByClusterContent(t *testing.T) {
	counting := &recordingValidator{}
	tr := validationTestTranslator(validator.NewCaching(counting, 0))
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	for range 5 {
		c, err := tr.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ucc, validationTestBackend("b1"))
		require.NoError(t, err)
		require.NotNil(t, c)
		require.NoError(t, tr.ValidateClusterConfig(context.Background(), c))
	}
	require.EqualValues(t, 1, counting.calls.Load(), "identical cluster content must be validated once")

	c, err := tr.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ucc, validationTestBackend("b2"))
	require.NoError(t, err)
	require.NoError(t, tr.ValidateClusterConfig(context.Background(), c))
	require.EqualValues(t, 2, counting.calls.Load(), "distinct cluster content must be validated separately")
}

func TestStrictValidationCachesInvalidVerdicts(t *testing.T) {
	counting := &recordingValidator{err: fmt.Errorf("%w: bad cluster", validator.ErrInvalidXDS)}
	tr := validationTestTranslator(validator.NewCaching(counting, 0))
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	for range 3 {
		c, err := tr.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ucc, validationTestBackend("b1"))
		require.NoError(t, err)
		require.NotNil(t, c)
		err = tr.ValidateClusterConfig(context.Background(), c)
		require.ErrorIs(t, err, validator.ErrInvalidXDS)
	}
	require.EqualValues(t, 1, counting.calls.Load(), "an invalid verdict must be cached by content")
}

func TestStrictValidationDoesNotCacheTransientErrors(t *testing.T) {
	counting := &recordingValidator{err: errors.New("exec: envoy binary not found")}
	tr := validationTestTranslator(validator.NewCaching(counting, 0))
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})

	c, err := tr.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ucc, validationTestBackend("b1"))
	require.NoError(t, err)
	err = tr.ValidateClusterConfig(context.Background(), c)
	require.Error(t, err)

	counting.err = nil
	c, err = tr.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ucc, validationTestBackend("b1"))
	require.NoError(t, err)
	require.NotNil(t, c)
	require.NoError(t, tr.ValidateClusterConfig(context.Background(), c))
	require.EqualValues(t, 2, counting.calls.Load(), "a transient failure must not be served from cache")
}

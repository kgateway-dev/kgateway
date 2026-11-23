package deployer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// staticValues is a HelmValuesGenerator that always returns the same values.
type staticValues map[string]any

func (v staticValues) GetValues(context.Context, client.Object) (map[string]any, error) {
	return v, nil
}

func (v staticValues) GetCacheSyncHandlers() []cache.InformerSynced {
	return nil
}

// TestGetObjsToDeployDoesNotLogValues guards against logging helm values, which can carry
// secrets that the chart renders into a Secret.
func TestGetObjsToDeployDoesNotLogValues(t *testing.T) {
	var logs bytes.Buffer
	defaultLogger := logger
	logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { logger = defaultLogger })

	const secret = "do-not-log-this-key"
	secretChart := &chart.Chart{
		Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "test", Version: "0.1.0"},
		Templates: []*chart.File{{
			Name: "templates/secret.yaml",
			Data: []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: test\nstringData:\n  key: {{ .Values.key }}\n"),
		}},
	}
	d := NewDeployer("test", runtime.NewScheme(), nil, secretChart, staticValues{"key": secret},
		func(client.Object) (string, string) { return "test", "default" })

	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}}
	objs, err := d.GetObjsToDeploy(t.Context(), gw)
	require.NoError(t, err)
	require.Len(t, objs, 1, "the chart should render the Secret")

	assert.Contains(t, logs.String(), "got deployer helm values", "the debug line should still be logged")
	assert.NotContains(t, logs.String(), secret)
}

// failingValues is a HelmValuesGenerator whose GetValues always fails.
type failingValues struct{}

func (failingValues) GetValues(context.Context, client.Object) (map[string]any, error) {
	return nil, errors.New("invalid parameters")
}

func (failingValues) GetCacheSyncHandlers() []cache.InformerSynced {
	return nil
}

// TestGetObjsToDeployErrorNamesKindStably guards the helm values error, which the Gateway
// status reports: it must name the object's kind, and must not depend on whether the object
// came with its TypeMeta set, which differs between informer paths.
func TestGetObjsToDeployErrorNamesKindStably(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, gwv1.Install(scheme))
	d := NewDeployer("test", scheme, nil, nil, failingValues{},
		func(client.Object) (string, string) { return "test", "default" })

	withoutTypeMeta := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"}}
	withTypeMeta := withoutTypeMeta.DeepCopy()
	withTypeMeta.SetGroupVersionKind(wellknown.GatewayGVK)

	_, errWithout := d.GetObjsToDeploy(t.Context(), withoutTypeMeta)
	_, errWith := d.GetObjsToDeploy(t.Context(), withTypeMeta)
	require.EqualError(t, errWithout,
		"failed to get helm values for object gateway.networking.k8s.io/v1, Kind=Gateway default/gw: invalid parameters")
	require.EqualError(t, errWith, errWithout.Error(), "the message must not depend on TypeMeta")
}

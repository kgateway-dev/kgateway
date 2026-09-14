//go:build e2e

package loadtesting

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidationMetricsDeltaClampsCounterResets(t *testing.T) {
	before := ValidationMetrics{
		Calls:            10,
		CacheHits:        9,
		CacheMisses:      8,
		Valid:            7,
		InvalidXDS:       6,
		InvocationErrors: 5,
		DurationCount:    4,
		DurationSeconds:  3.5,
		ByCaller: map[string]ValidationCallerMetrics{
			"route_full": {
				Calls:            10,
				CacheHits:        9,
				CacheMisses:      8,
				Valid:            7,
				InvalidXDS:       6,
				InvocationErrors: 5,
				DurationCount:    4,
				DurationSeconds:  3.5,
			},
		},
	}
	after := ValidationMetrics{
		Calls:            1,
		CacheHits:        1,
		CacheMisses:      1,
		Valid:            1,
		InvalidXDS:       1,
		InvocationErrors: 1,
		DurationCount:    1,
		DurationSeconds:  1,
		ByCaller: map[string]ValidationCallerMetrics{
			"route_full": {
				Calls:            1,
				CacheHits:        1,
				CacheMisses:      1,
				Valid:            1,
				InvalidXDS:       1,
				InvocationErrors: 1,
				DurationCount:    1,
				DurationSeconds:  1,
			},
		},
	}

	delta := after.Delta(before)

	assert.Zero(t, delta.Calls)
	assert.Zero(t, delta.CacheHits)
	assert.Zero(t, delta.CacheMisses)
	assert.Zero(t, delta.Valid)
	assert.Zero(t, delta.InvalidXDS)
	assert.Zero(t, delta.InvocationErrors)
	assert.Zero(t, delta.DurationCount)
	assert.Zero(t, delta.DurationSeconds)

	caller := delta.ByCaller["route_full"]
	assert.Zero(t, caller.Calls)
	assert.Zero(t, caller.CacheHits)
	assert.Zero(t, caller.CacheMisses)
	assert.Zero(t, caller.Valid)
	assert.Zero(t, caller.InvalidXDS)
	assert.Zero(t, caller.InvocationErrors)
	assert.Zero(t, caller.DurationCount)
	assert.Zero(t, caller.DurationSeconds)
}

func TestFleetReadinessRequiresEveryLiveStream(t *testing.T) {
	clients := make([]*syntheticClient, 100)
	for i := range clients {
		clients[i] = &syntheticClient{done: make(chan struct{})}
		if i < 34 {
			clients[i].acks.Store(3)
		}
	}
	assert.Equal(t, 34, servedStreams(clients), "multiple resource ACKs must not count as additional served streams")
	for _, c := range clients {
		c.acks.Store(1)
	}
	assert.Equal(t, 100, servedStreams(clients))
	close(clients[0].done)
	assert.Equal(t, 99, servedStreams(clients), "terminated streams must not count as served")
	assert.Zero(t, servedStreams([]*syntheticClient{{done: make(chan struct{})}}), "older streams cannot satisfy a new stream's readiness")
}

func TestBenchConvergenceRequiresQuietWindow(t *testing.T) {
	oldFleetTimeout, oldCostTimeout := fleetIterTimeout, benchIterationTimeout
	oldFleetSettle, oldCostSettle := fleetSettleMillis, benchSettleMillis
	t.Cleanup(func() {
		fleetIterTimeout, benchIterationTimeout = oldFleetTimeout, oldCostTimeout
		fleetSettleMillis, benchSettleMillis = oldFleetSettle, oldCostSettle
	})
	fleetIterTimeout, benchIterationTimeout = 300*time.Millisecond, 300*time.Millisecond
	fleetSettleMillis, benchSettleMillis = 1000, 1000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "kgateway_xds_snapshot_transforms_total 1")
	}))
	defer server.Close()
	fleet := &XdsFleetSuite{metricsURL: server.URL}
	fleet.SetT(t)
	_, ok := fleet.waitConverged(0)
	assert.False(t, ok, "a transform without a full quiet window must time out")
	cost := &XdsCostSuite{metricsURL: server.URL}
	cost.SetT(t)
	_, ok = cost.waitConverged(0, time.Now())
	assert.False(t, ok, "cost suite must also require a full quiet window")
}

func TestBenchmarkEnvRestoresValueSources(t *testing.T) {
	original := map[string]*corev1.EnvVar{
		"SECRET":  {Name: "SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "secret"}}},
		"CONFIG":  {Name: "CONFIG", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "config"}}},
		"FIELD":   {Name: "FIELD", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		"LITERAL": {Name: "LITERAL", Value: "original"},
		"ADDED":   nil,
	}
	current := []corev1.EnvVar{{Name: "SECRET", Value: "override"}, {Name: "ADDED", Value: "temporary"}, {Name: "UNRELATED", Value: "keep"}}
	restored := restoredBenchmarkEnv(current, original)
	assert.Len(t, restored, 5)
	assert.Contains(t, restored, current[2])
	for _, env := range original {
		if env != nil {
			assert.Contains(t, restored, *env)
		}
	}
}

func TestBenchmarkResourceRestoration(t *testing.T) {
	for _, original := range []corev1.ResourceRequirements{
		{},
		{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")}},
		{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")}, Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")}},
	} {
		scheme := runtime.NewScheme()
		require.NoError(t, appsv1.AddToScheme(scheme))
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "test"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "controller", Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
					Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
				}},
				{Name: "sidecar", Image: "unchanged"},
			}}}},
		}
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		require.NoError(t, updateBenchmarkContainer(context.Background(), kube, "test", "controller", "controller", func(c *corev1.Container) {
			c.Resources = *original.DeepCopy()
		}))
		var restored appsv1.Deployment
		require.NoError(t, kube.Get(context.Background(), client.ObjectKeyFromObject(deployment), &restored))
		assert.Equal(t, original, restored.Spec.Template.Spec.Containers[0].Resources)
		assert.Equal(t, deployment.Spec.Template.Spec.Containers[1], restored.Spec.Template.Spec.Containers[1])
	}
}

package krtcollections

import (
	"context"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// validationAnnotation returns a pod's validation annotation for subset and ports.
func validationAnnotation(t *testing.T, subset string, ports ...uint32) string {
	t.Helper()

	data, err := bootstrappatch.EncodeValidationData(subset, ports, nil)
	require.NoError(t, err)
	return data
}

// The bootstrap a client is held for must be the one GatewayRunningBootstraps adds
// to the Gateway's validation bootstraps for the same pod, or the client is never
// released.
func TestPodBootstrapMatchesRunningBootstraps(t *testing.T) {
	gw := types.NamespacedName{Namespace: "default", Name: "gw"}
	tests := map[string]struct {
		annotation *string
		want       *ir.PodBootstrap
	}{
		"a pod without the annotation runs Envoy's defaults": {
			want: &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()},
		},
		"a patched subset": {
			annotation: new(validationAnnotation(t, raisedSubset, 8083, 9000)),
			want:       &ir.PodBootstrap{Subset: raisedSubset, Ports: []uint32{8083, 9000}},
		},
		"a subset that sets nothing is Envoy's defaults": {
			annotation: new(validationAnnotation(t, bootstrappatch.EmptyValidationSubset, 8083)),
			want:       &ir.PodBootstrap{Ports: []uint32{8083}},
		},
		"an annotation that does not decode is Envoy's defaults on the default ports": {
			annotation: new("not json"),
			want:       &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()},
		},
		"a subset that does not parse is Envoy's defaults on the annotation's ports": {
			annotation: new(validationAnnotation(t, `{"layered_runtim":{}}`, 8083)),
			want:       &ir.PodBootstrap{Ports: []uint32{8083}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Give the pod the annotation, if any.
			annotations := map[string]string{}
			if tc.annotation != nil {
				annotations[bootstrappatch.ValidationAnnotation] = *tc.annotation
			}

			// Read the pod's bootstrap as the client does: nil without the
			// annotation, which the client treats as Envoy's defaults.
			client := podBootstrapFromAnnotations(annotations)
			if tc.annotation == nil {
				assert.Nil(t, client, "a pod without the annotation records no bootstrap")
				client = defaultPodBootstrap()
			}
			assert.Equal(t, tc.want, client)

			// GatewayRunningBootstraps reads the same subset and ports.
			running := runningBootstraps(gw, []WrappedPod{{Named: krt.Named{Name: "gw-a", Namespace: "default"}, Annotations: annotations}})
			require.Len(t, running, 1)
			assert.Equal(t, tc.want.Subset, running[0].subset.Subset, "the subset GatewayRunningBootstraps validates under")
			assert.Equal(t, tc.want.Ports, running[0].ports, "the ports GatewayRunningBootstraps reserves")
		})
	}
}

// LocalityPod records the pod's bootstrap, and Equals compares it.
func TestLocalityPodBootstrap(t *testing.T) {
	// Build the pod collection over a pod with the validation annotation.
	annotation := validationAnnotation(t, raisedSubset, 8083)
	mock := krttest.NewMock(t, []any{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "gw-a", Namespace: "default",
		Annotations: map[string]string{bootstrappatch.ValidationAnnotation: annotation},
	}}})
	nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
	namespaces := NewNamespaceCollectionFromCol(context.Background(), krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})
	network := NewSystemNamespaceNetwork(namespaces, "istio-system", krtutil.KrtOptions{})
	pods := NewLocalityPodsCollection(nodes, network, krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
	pods.WaitUntilSynced(context.Background().Done())

	// The pod's bootstrap is recorded.
	lp := pods.List()[0]
	assert.Equal(t, &ir.PodBootstrap{Subset: raisedSubset, Ports: []uint32{8083}}, lp.Bootstrap)

	// A different bootstrap, or none, makes the pods differ.
	other := lp
	other.Bootstrap = &ir.PodBootstrap{Subset: loweredSubset, Ports: []uint32{8083}}
	assert.False(t, lp.Equals(other), "a different subset must make the pods differ")
	other.Bootstrap = nil
	assert.False(t, lp.Equals(other), "a pod without a bootstrap must differ from one with it")
}

// The client takes the bootstrap of the pod it connects from: Envoy's defaults
// for a pod without the annotation, and none without pod locality.
func TestUniqueClientBootstrap(t *testing.T) {
	t.Cleanup(SetXdsFirstConnectDelayForTest(0))
	role := wellknown.GatewayApiProxyValue + "~default~gw"
	request := func(pod string) *envoy_service_discovery_v3.DiscoveryRequest {
		return &envoy_service_discovery_v3.DiscoveryRequest{Node: &envoycorev3.Node{
			Id:       pod + ".default",
			Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{xds.RoleKey: structpb.NewStringValue(role)}},
		}}
	}
	patched := &ir.PodBootstrap{Subset: raisedSubset, Ports: []uint32{8083}}
	tests := map[string]struct {
		pods []LocalityPod
		want *ir.PodBootstrap
	}{
		"a pod without the annotation has Envoy's defaults": {
			pods: []LocalityPod{{Named: krt.Named{Name: "gw-a", Namespace: "default"}, AugmentedLabels: map[string]string{"pod-template-hash": "a"}}},
			want: &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()},
		},
		"a patched pod has its bootstrap": {
			pods: []LocalityPod{{Named: krt.Named{Name: "gw-a", Namespace: "default"}, AugmentedLabels: map[string]string{"pod-template-hash": "b"}, Bootstrap: patched}},
			want: patched,
		},
		"a client without pod locality has none": {},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Connect a client, with pod locality when there are pods.
			ctx := context.Background()
			var pods krt.Collection[LocalityPod]
			if tc.pods != nil {
				pods = krt.NewStaticCollection(nil, tc.pods)
			}
			cb, builder, _ := NewUniquelyConnectedClients(false)
			clients := builder(ctx, krtutil.KrtOptions{}, pods)
			clients.WaitUntilSynced(ctx.Done())
			require.NoError(t, cb.OnStreamRequest(1, request("gw-a")))

			// The client carries the pod's bootstrap.
			require.Eventually(t, func() bool { return len(clients.List()) == 1 }, time.Second, 10*time.Millisecond)
			assert.Equal(t, tc.want, clients.List()[0].Bootstrap)
		})
	}
}

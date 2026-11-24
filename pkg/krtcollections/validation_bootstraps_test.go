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
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

const (
	raisedSubset  = `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":1000}}]}}`
	loweredSubset = `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":10}}]}}`
)

// validationSummary is a Gateway's validation subset and reserved ports, without the
// parsed subset.
type validationSummary struct {
	Subset        string
	ReservedPorts []uint32
}

func summarize(vb *ir.GatewayValidationBootstraps) *validationSummary {
	if vb == nil {
		return nil
	}
	return &validationSummary{Subset: vb.Subset.Subset, ReservedPorts: vb.ReservedPorts}
}

// bootstrapConfigMap returns the generated bootstrap ConfigMap of the Gateway gw, with
// validation data for subset and ports.
func bootstrapConfigMap(t *testing.T, gw, subset string, ports ...uint32) *corev1.ConfigMap {
	t.Helper()

	data, err := bootstrapoverlay.EncodeValidationData(subset, ports)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: gw, Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: gwv1.GroupVersion.String(), Kind: wellknown.GatewayKind, Name: gw}},
		},
		Data: map[string]string{"envoy.yaml": "{}", bootstrapoverlay.ValidationKey: data},
	}
}

// proxyPod returns a proxy pod of the Gateway gw that started from the bootstrap with
// checksum sum.
func proxyPod(name, gw, sum string) WrappedPod {
	return WrappedPod{
		Named:       krt.Named{Name: name, Namespace: "default"},
		Labels:      map[string]string{wellknown.GatewayNameLabel: gw},
		Annotations: map[string]string{bootstrapoverlay.ChecksumAnnotation: sum},
	}
}

func TestGatewayValidation(t *testing.T) {
	g := NewWithT(t)

	// Build the Gateway index over a Gateway with no ConfigMap yet.
	config := getConfig(t)
	config.GatewayClasses = krt.NewStaticCollection(nil, []*gwv1.GatewayClass{{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(config.EnvoyControllerName)},
	}}, config.KrtOpts.ToOptions("GatewayClasses")...)
	config.Gateways = krt.NewStaticCollection(nil, []*gwv1.Gateway{testGateway("gw")}, config.KrtOpts.ToOptions("Gateways")...)
	configMaps := krt.NewMutableCollection[*corev1.ConfigMap](nil, nil, config.KrtOpts.ToOptions("ConfigMaps")...)
	config.ConfigMaps = configMaps.AsCollection()
	index := NewGatewayIndex(config)
	g.Expect(index.Gateways.WaitUntilSynced(t.Context().Done())).To(BeTrue())
	g.Expect(index.Validation.WaitUntilSynced(t.Context().Done())).To(BeTrue())
	validation := func() *ir.GatewayValidationBootstraps {
		gw := index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: "gw"}.ResourceName())
		if gw == nil {
			return nil
		}
		return gw.ValidationBootstraps
	}
	summary := func() *validationSummary { return summarize(validation()) }

	// Without validation data, the Gateway validates against Envoy's defaults.
	g.Eventually(func() *GatewayValidation { return index.Validation.GetKey("default/gw") }).ShouldNot(BeNil())
	g.Consistently(validation).Should(BeNil(), "a Gateway without overlays validates against Envoy's defaults")

	// A ConfigMap without the Gateway as owner is not the Gateway's.
	unowned := bootstrapConfigMap(t, "gw", raisedSubset, 8083, 9000)
	unowned.OwnerReferences = nil
	configMaps.UpdateObject(unowned)
	g.Consistently(validation).Should(BeNil(), "a ConfigMap of no Gateway should not apply")

	// The Gateway's ConfigMap gives the subset and the reserved ports, and the parsed
	// subset is kept for validation.
	configMaps.UpdateObject(bootstrapConfigMap(t, "gw", raisedSubset, 8083, 9000))
	g.Eventually(summary).Should(Equal(&validationSummary{Subset: raisedSubset, ReservedPorts: []uint32{8083, 9000}}))
	g.Expect(validation().Subset.Bootstrap.GetLayeredRuntime().GetLayers()).To(HaveLen(1))

	// A new subset replaces the old one: only the current bootstrap is validated against.
	configMaps.UpdateObject(bootstrapConfigMap(t, "gw", loweredSubset, 8083))
	g.Eventually(summary).Should(Equal(&validationSummary{Subset: loweredSubset, ReservedPorts: []uint32{8083}}))

	// When the overlays are removed, the ConfigMap loses the key, and the Gateway
	// validates against Envoy's defaults again.
	cm := bootstrapConfigMap(t, "gw", bootstrapoverlay.EmptyValidationSubset)
	delete(cm.Data, bootstrapoverlay.ValidationKey)
	configMaps.UpdateObject(cm)
	g.Eventually(validation).Should(BeNil())
}

func TestGatewayRunningBootstraps(t *testing.T) {
	g := NewWithT(t)

	// Build the Gateway index over a Gateway with no pods yet.
	config := getConfig(t)
	config.GatewayClasses = krt.NewStaticCollection(nil, []*gwv1.GatewayClass{{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(config.EnvoyControllerName)},
	}}, config.KrtOpts.ToOptions("GatewayClasses")...)
	config.Gateways = krt.NewStaticCollection(nil, []*gwv1.Gateway{testGateway("gw")}, config.KrtOpts.ToOptions("Gateways")...)
	pods := krt.NewMutableCollection[WrappedPod](nil, nil, config.KrtOpts.ToOptions("Pods")...)
	config.Pods = pods.AsCollection()
	index := NewGatewayIndex(config)
	g.Expect(index.RunningBootstraps.WaitUntilSynced(t.Context().Done())).To(BeTrue())
	revisions := func() []string {
		rb := index.RunningBootstraps.GetKey("default/gw")
		if rb == nil {
			return []string{"missing"}
		}
		return rb.Revisions
	}

	// Without pods, no revision is running.
	g.Eventually(revisions).Should(BeEmpty())

	// Pods of another Gateway are not the Gateway's.
	pods.UpdateObject(proxyPod("other-a", "other", "sum-other"))
	g.Consistently(revisions).Should(BeEmpty(), "pods of another Gateway should not apply")

	// The Gateway's pods give their distinct checksums, sorted. An exited pod restarts
	// no container and is left out.
	pods.UpdateObject(proxyPod("gw-b", "gw", "sum-old"))
	pods.UpdateObject(proxyPod("gw-a", "gw", "sum-new"))
	pods.UpdateObject(proxyPod("gw-c", "gw", "sum-new"))
	exited := proxyPod("gw-d", "gw", "sum-exited")
	exited.Terminal = true
	pods.UpdateObject(exited)
	g.Eventually(revisions).Should(Equal([]string{"sum-new", "sum-old"}))

	// Once every pod has rolled to the current bootstrap, only its revision remains.
	pods.UpdateObject(proxyPod("gw-b", "gw", "sum-new"))
	g.Eventually(revisions).Should(Equal([]string{"sum-new"}))
}

func TestGatewayValidationEquals(t *testing.T) {
	// Build a value with every compared field set.
	newValidation := func() GatewayValidation {
		return GatewayValidation{
			Gateway: types.NamespacedName{Namespace: "default", Name: "gw"},
			Validation: &ir.GatewayValidationBootstraps{
				Subset:        ir.ValidationBootstrap{Subset: raisedSubset},
				ReservedPorts: []uint32{9000},
			},
		}
	}

	// Each case changes one field of a second value.
	tests := []struct {
		name   string
		change func(g *GatewayValidation)
		want   bool
	}{
		{name: "same values", change: func(*GatewayValidation) {}, want: true},
		{name: "another Gateway", change: func(g *GatewayValidation) { g.Gateway.Name = "other" }},
		{name: "another subset", change: func(g *GatewayValidation) { g.Validation.Subset.Subset = loweredSubset }},
		{name: "another reserved port", change: func(g *GatewayValidation) { g.Validation.ReservedPorts = []uint32{9001} }},
		{name: "no validation data", change: func(g *GatewayValidation) { g.Validation = nil }},
	}

	// Compare in both directions, so a nil on either side is covered.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, changed := newValidation(), newValidation()
			tt.change(&changed)
			assert.Equal(t, tt.want, base.Equals(changed))
			assert.Equal(t, tt.want, changed.Equals(base))
		})
	}

	// Two values without validation data are equal.
	withoutData := GatewayValidation{Gateway: types.NamespacedName{Namespace: "default", Name: "gw"}}
	assert.True(t, withoutData.Equals(withoutData), "two values without validation data should be equal")
}

func TestGatewayRunningBootstrapsEquals(t *testing.T) {
	// Build a value with every compared field set.
	newRunning := func() GatewayRunningBootstraps {
		return GatewayRunningBootstraps{
			Gateway:   types.NamespacedName{Namespace: "default", Name: "gw"},
			Revisions: []string{"sum-a", "sum-b"},
		}
	}

	// Each case changes one field of a second value.
	tests := []struct {
		name   string
		change func(g *GatewayRunningBootstraps)
		want   bool
	}{
		{name: "same values", change: func(*GatewayRunningBootstraps) {}, want: true},
		{name: "another Gateway", change: func(g *GatewayRunningBootstraps) { g.Gateway.Namespace = "other" }},
		{name: "another revision", change: func(g *GatewayRunningBootstraps) { g.Revisions[1] = "sum-c" }},
		{name: "one revision fewer", change: func(g *GatewayRunningBootstraps) { g.Revisions = g.Revisions[:1] }},
		{name: "no revisions", change: func(g *GatewayRunningBootstraps) { g.Revisions = nil }},
	}

	// Compare in both directions.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, changed := newRunning(), newRunning()
			tt.change(&changed)
			assert.Equal(t, tt.want, base.Equals(changed))
			assert.Equal(t, tt.want, changed.Equals(base))
		})
	}
}

func TestCurrentValidationData(t *testing.T) {
	gw := types.NamespacedName{Namespace: "default", Name: "gw"}

	// Without a ConfigMap, Envoy's defaults and the default reserved ports apply.
	assert.Nil(t, currentValidationData(gw, nil))

	// Validation data that does not decode leaves Envoy's defaults and the default
	// reserved ports.
	broken := bootstrapConfigMap(t, "gw", raisedSubset, 9000)
	broken.Data[bootstrapoverlay.ValidationKey] = "not json"
	assert.Nil(t, currentValidationData(gw, []*corev1.ConfigMap{broken}))

	// A subset that sets nothing, with no extra ports, needs no validation bootstraps.
	assert.Nil(t, currentValidationData(gw, []*corev1.ConfigMap{bootstrapConfigMap(t, "gw", bootstrapoverlay.EmptyValidationSubset)}))

	// Reserved ports alone still apply.
	assert.Equal(t, &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}},
		currentValidationData(gw, []*corev1.ConfigMap{bootstrapConfigMap(t, "gw", bootstrapoverlay.EmptyValidationSubset, 9000)}))

	// A subset that does not parse is Envoy's defaults, and its ports still apply.
	assert.Equal(t, &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}},
		currentValidationData(gw, []*corev1.ConfigMap{bootstrapConfigMap(t, "gw", `{"layered_runtim":{}}`, 9000)}))

	// With two ConfigMaps that have the key, the first by name is used.
	other := bootstrapConfigMap(t, "gw", loweredSubset, 7000)
	other.Name = "a-other"
	got := summarize(currentValidationData(gw, []*corev1.ConfigMap{bootstrapConfigMap(t, "gw", raisedSubset, 9000), other}))
	assert.Equal(t, &validationSummary{Subset: loweredSubset, ReservedPorts: []uint32{7000}}, got)
}

// validationAnnotation returns a pod's validation annotation for subset and ports.
func validationAnnotation(t *testing.T, subset string, ports ...uint32) string {
	t.Helper()

	data, err := bootstrapoverlay.EncodeValidationData(subset, ports)
	require.NoError(t, err)
	return data
}

// A client is released from the first-snapshot hold once the Gateway's validation
// subset is the one its pod started with, so a pod annotation must read as the same
// subset as the ConfigMap key it was copied from.
func TestPodBootstrapMatchesValidation(t *testing.T) {
	gw := types.NamespacedName{Namespace: "default", Name: "gw"}
	tests := map[string]struct {
		annotation *string
		want       *ir.PodBootstrap
	}{
		"a pod without the annotation runs Envoy's defaults": {
			want: &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()},
		},
		"a merged subset": {
			annotation: new(validationAnnotation(t, raisedSubset, 8083, 9000)),
			want:       &ir.PodBootstrap{Subset: raisedSubset, Ports: []uint32{8083, 9000}},
		},
		"a subset that sets nothing is Envoy's defaults": {
			annotation: new(validationAnnotation(t, bootstrapoverlay.EmptyValidationSubset, 8083)),
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
				annotations[bootstrapoverlay.ValidationAnnotation] = *tc.annotation
			}

			// Read the pod's bootstrap as the client does: nil without the
			// annotation, which the client treats as Envoy's defaults.
			client := podBootstrapFromAnnotations(annotations)
			if tc.annotation == nil {
				assert.Nil(t, client, "a pod without the annotation records no bootstrap")
				client = defaultPodBootstrap()
			}
			assert.Equal(t, tc.want, client)

			// The ConfigMap key with the same data gives the same subset.
			if tc.annotation == nil {
				return
			}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Data:       map[string]string{bootstrapoverlay.ValidationKey: *tc.annotation},
			}
			assert.Equal(t, tc.want.Subset, currentValidationData(gw, []*corev1.ConfigMap{cm}).GetSubset().Subset,
				"the subset the Gateway validates under")
		})
	}
}

// LocalityPod records the pod's bootstrap, and Equals compares it.
func TestLocalityPodBootstrap(t *testing.T) {
	// Build the pod collection over a pod with the validation annotation.
	annotation := validationAnnotation(t, raisedSubset, 8083)
	mock := krttest.NewMock(t, []any{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "gw-a", Namespace: "default",
		Annotations: map[string]string{bootstrapoverlay.ValidationAnnotation: annotation},
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
	withOverlay := &ir.PodBootstrap{Subset: raisedSubset, Ports: []uint32{8083}}
	tests := map[string]struct {
		pods []LocalityPod
		want *ir.PodBootstrap
	}{
		"a pod without the annotation has Envoy's defaults": {
			pods: []LocalityPod{{Named: krt.Named{Name: "gw-a", Namespace: "default"}, AugmentedLabels: map[string]string{"pod-template-hash": "a"}}},
			want: &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()},
		},
		"a pod with an overlay has its bootstrap": {
			pods: []LocalityPod{{Named: krt.Named{Name: "gw-a", Namespace: "default"}, AugmentedLabels: map[string]string{"pod-template-hash": "b"}, Bootstrap: withOverlay}},
			want: withOverlay,
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

package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	raisedSubset  = `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":1000}}]}}`
	loweredSubset = `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":10}}]}}`
)

// runningSummary is what a Gateway's running bootstraps say, without the parsed
// subsets.
type runningSummary struct {
	Subsets          []ir.ValidationBootstrap
	ReservedPorts    []uint32
	PodReservedPorts []ir.PodReservedPort
	StaticClusters   []string
	PodPorts         []uint32
	Revisions        []string
}

func summarize(rb *GatewayRunningBootstraps) *runningSummary {
	if rb == nil {
		return nil
	}
	out := &runningSummary{PodPorts: rb.PodPorts, Revisions: rb.Revisions}
	if vb := rb.Validation; vb != nil {
		out.ReservedPorts = vb.ReservedPorts
		out.PodReservedPorts = vb.PodReservedPorts
		out.StaticClusters = vb.StaticClusters
		for _, s := range vb.Subsets {
			out.Subsets = append(out.Subsets, ir.ValidationBootstrap{Subset: s.Subset, Pod: s.Pod})
		}
	}
	return out
}

// bootstrapConfigMap returns the generated bootstrap ConfigMap of the Gateway gw,
// with validation data for subset and ports, and the static cluster xds_cluster.
func bootstrapConfigMap(t *testing.T, gw, subset string, ports ...uint32) *corev1.ConfigMap {
	t.Helper()
	data, err := bootstrappatch.EncodeValidationData(subset, ports, []string{"xds_cluster"})
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: gw, Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: gwv1.GroupVersion.String(), Kind: wellknown.GatewayKind, Name: gw}},
		},
		Data: map[string]string{"envoy.yaml": "{}", bootstrappatch.ValidationKey: data},
	}
}

// proxyPod returns a proxy pod of the Gateway gw that started from the bootstrap with
// checksum sum, whose validation annotation holds subset and ports. An empty subset
// leaves the annotation off.
func proxyPod(t *testing.T, name, gw, sum, subset string, ports ...uint32) WrappedPod {
	t.Helper()

	// Every pod carries the checksum of its bootstrap.
	pod := WrappedPod{
		Named:       krt.Named{Name: name, Namespace: "default"},
		Labels:      map[string]string{wellknown.GatewayNameLabel: gw},
		Annotations: map[string]string{bootstrappatch.ChecksumAnnotation: sum},
	}

	// A pod started from a patched bootstrap also carries its validation data.
	if subset != "" {
		data, err := bootstrappatch.EncodeValidationData(subset, ports, nil)
		if err != nil {
			t.Fatal(err)
		}
		pod.Annotations[bootstrappatch.ValidationAnnotation] = data
	}
	return pod
}

func TestGatewayRunningBootstraps(t *testing.T) {
	g := NewWithT(t)

	// Build the Gateway index over a Gateway with no ConfigMap and no pods yet.
	config := getConfig(t)
	config.GatewayClasses = krt.NewStaticCollection(nil, []*gwv1.GatewayClass{{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(config.EnvoyControllerName)},
	}}, config.KrtOpts.ToOptions("GatewayClasses")...)
	config.Gateways = krt.NewStaticCollection(nil, []*gwv1.Gateway{testGateway("gw")}, config.KrtOpts.ToOptions("Gateways")...)
	configMaps := krt.NewMutableCollection[*corev1.ConfigMap](nil, nil, config.KrtOpts.ToOptions("ConfigMaps")...)
	pods := krt.NewMutableCollection[WrappedPod](nil, nil, config.KrtOpts.ToOptions("Pods")...)
	config.ConfigMaps = configMaps.AsCollection()
	config.Pods = pods.AsCollection()
	index := NewGatewayIndex(config)
	g.Expect(index.Gateways.WaitUntilSynced(t.Context().Done())).To(BeTrue())
	g.Expect(index.RunningBootstraps.WaitUntilSynced(t.Context().Done())).To(BeTrue())
	summary := func() *runningSummary {
		return summarize(index.RunningBootstraps.GetKey("default/gw"))
	}
	validation := func() *ir.GatewayValidationBootstraps {
		gw := index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: "gw"}.ResourceName())
		if gw == nil {
			return nil
		}
		return gw.ValidationBootstraps
	}

	// Without validation data or pods, every proxy runs Envoy's defaults.
	g.Eventually(summary).Should(Equal(&runningSummary{}))
	g.Consistently(validation).Should(BeNil(), "a Gateway without patches validates against Envoy's defaults")

	// A ConfigMap without the Gateway as owner, and pods of another Gateway, are not
	// the Gateway's.
	unowned := bootstrapConfigMap(t, "gw", raisedSubset, 8083, 9000)
	unowned.OwnerReferences = nil
	configMaps.UpdateObject(unowned)
	pods.UpdateObject(proxyPod(t, "other-a", "other", "sum-other", raisedSubset, 8083))
	g.Consistently(summary).Should(Equal(&runningSummary{}), "objects of no Gateway or another Gateway should not apply")

	// The Gateway's ConfigMap gives the current subset, the reserved ports and the
	// static cluster names.
	configMaps.UpdateObject(bootstrapConfigMap(t, "gw", raisedSubset, 8083, 9000))
	g.Eventually(summary).Should(Equal(&runningSummary{
		Subsets:        []ir.ValidationBootstrap{{Subset: raisedSubset}},
		ReservedPorts:  []uint32{8083, 9000},
		StaticClusters: []string{"xds_cluster"},
	}))

	// A pod started before the patches, without the annotation, adds Envoy's defaults
	// and reserves the default ports the current bootstrap no longer listens on, named
	// for the first such pod. An exited pod receives no configuration and is left out.
	pods.UpdateObject(proxyPod(t, "gw-b", "gw", "sum-old", ""))
	pods.UpdateObject(proxyPod(t, "gw-a", "gw", "sum-old", ""))
	exited := proxyPod(t, "gw-c", "gw", "sum-exited", loweredSubset, 7000)
	exited.Terminal = true
	pods.UpdateObject(exited)
	g.Eventually(summary).Should(Equal(&runningSummary{
		Subsets:          []ir.ValidationBootstrap{{Subset: raisedSubset}, {Pod: "gw-a"}},
		ReservedPorts:    []uint32{8083, 9000},
		PodReservedPorts: []ir.PodReservedPort{{Port: 8082, Pod: "gw-a"}, {Port: 9091, Pod: "gw-a"}, {Port: 19000, Pod: "gw-a"}},
		StaticClusters:   []string{"xds_cluster"},
		PodPorts:         []uint32{8082, 9091, 19000},
		Revisions:        []string{"sum-old"},
	}))

	// The translator sees the same validation bootstraps on the Gateway.
	g.Eventually(func() []ir.PodReservedPort { return validation().GetPodReservedPorts() }).Should(HaveLen(3))

	// Once every pod has rolled to the current bootstrap, only it remains.
	pods.UpdateObject(proxyPod(t, "gw-a", "gw", "sum-new", raisedSubset, 8083, 9000))
	pods.UpdateObject(proxyPod(t, "gw-b", "gw", "sum-new", raisedSubset, 8083, 9000))
	g.Eventually(summary).Should(Equal(&runningSummary{
		Subsets:        []ir.ValidationBootstrap{{Subset: raisedSubset}},
		ReservedPorts:  []uint32{8083, 9000},
		StaticClusters: []string{"xds_cluster"},
		PodPorts:       []uint32{8083, 9000},
		Revisions:      []string{"sum-new"},
	}))

	// When the patches are removed, the ConfigMap loses the key, and the pods still
	// running the raised limit and listening on 9000 keep both in validation until
	// they roll.
	cm := bootstrapConfigMap(t, "gw", bootstrappatch.EmptyValidationSubset)
	delete(cm.Data, bootstrappatch.ValidationKey)
	configMaps.UpdateObject(cm)
	g.Eventually(summary).Should(Equal(&runningSummary{
		Subsets:          []ir.ValidationBootstrap{{}, {Subset: raisedSubset, Pod: "gw-a"}},
		PodReservedPorts: []ir.PodReservedPort{{Port: 8083, Pod: "gw-a"}, {Port: 9000, Pod: "gw-a"}},
		PodPorts:         []uint32{8083, 9000},
		Revisions:        []string{"sum-new"},
	}))

	// The parsed subsets are kept for validation.
	vb := validation()
	g.Expect(vb.Subsets[0].Bootstrap).To(BeNil(), "Envoy's defaults need no subset")
	g.Expect(vb.Subsets[1].Bootstrap.GetLayeredRuntime().GetLayers()).To(HaveLen(1))
}

func TestBuildValidationBootstraps(t *testing.T) {
	gw := types.NamespacedName{Namespace: "default", Name: "gw"}

	// A subset that sets nothing is Envoy's defaults.
	assert.Equal(t, ir.ValidationBootstrap{Pod: "gw-a"}, parseValidationSubset(gw, bootstrappatch.EmptyValidationSubset, "gw-a"))

	// A subset that does not parse is treated as Envoy's defaults.
	assert.Equal(t, ir.ValidationBootstrap{Pod: "gw-a"}, parseValidationSubset(gw, `{"layered_runtim":{}}`, "gw-a"))

	// Validation data that does not decode leaves Envoy's defaults and the default
	// reserved ports.
	broken := bootstrapConfigMap(t, "gw", raisedSubset, 9000)
	broken.Data[bootstrappatch.ValidationKey] = "not json"
	assert.Equal(t, currentValidation{}, currentValidationData(gw, []*corev1.ConfigMap{broken}))

	// A pod annotation that does not decode counts as Envoy's defaults and the default
	// ports.
	pod := proxyPod(t, "gw-a", "gw", "sum", raisedSubset, 9000)
	pod.Annotations[bootstrappatch.ValidationAnnotation] = "not json"
	assert.Equal(t, podBootstraps{{subset: ir.ValidationBootstrap{Pod: "gw-a"}, ports: []uint32{8082, 9091, 19000}, checksum: "sum"}},
		runningBootstraps(gw, []WrappedPod{pod}))

	// Only the default reserved ports and Envoy's defaults need no validation
	// bootstraps.
	defaults := podBootstraps{{subset: ir.ValidationBootstrap{Pod: "gw-a"}, ports: []uint32{8082, 9091, 19000}}}
	assert.Nil(t, buildValidationBootstraps(currentValidation{}, defaults))

	// Reserved ports alone still apply, as do static cluster names alone.
	assert.Equal(t, &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}},
		buildValidationBootstraps(currentValidation{reservedPorts: []uint32{9000}}, nil))
	assert.Equal(t, &ir.GatewayValidationBootstraps{StaticClusters: []string{"xds_cluster"}},
		buildValidationBootstraps(currentValidation{staticClusters: []string{"xds_cluster"}}, nil))

	// A port only a running pod listens on is reserved for that pod, the first by name.
	running := podBootstraps{
		{subset: ir.ValidationBootstrap{Pod: "gw-a"}, ports: []uint32{7000, 8082}},
		{subset: ir.ValidationBootstrap{Pod: "gw-b"}, ports: []uint32{7000}},
	}
	assert.Equal(t, &ir.GatewayValidationBootstraps{PodReservedPorts: []ir.PodReservedPort{{Port: 7000, Pod: "gw-a"}}},
		buildValidationBootstraps(currentValidation{}, running))
}

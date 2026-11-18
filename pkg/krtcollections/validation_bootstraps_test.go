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

// validationSummary is what a Gateway's validation bootstraps say, without the parsed
// subsets.
type validationSummary struct {
	Subsets       []ir.ValidationBootstrap
	ReservedPorts []uint32
}

func summarize(vb *ir.GatewayValidationBootstraps) *validationSummary {
	if vb == nil {
		return nil
	}
	out := &validationSummary{ReservedPorts: vb.ReservedPorts}
	for _, s := range vb.Subsets {
		out.Subsets = append(out.Subsets, ir.ValidationBootstrap{Subset: s.Subset, Pod: s.Pod})
	}
	return out
}

// bootstrapConfigMap returns the generated bootstrap ConfigMap of the Gateway gw,
// with validation data for subset and ports.
func bootstrapConfigMap(t *testing.T, gw, subset string, ports ...uint32) *corev1.ConfigMap {
	t.Helper()
	data, err := bootstrappatch.EncodeValidationData(subset, ports)
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

// proxyPod returns a proxy pod of the Gateway gw that started with subset; an empty
// subset leaves the annotation off.
func proxyPod(name, gw, subset string) WrappedPod {
	pod := WrappedPod{
		Named:  krt.Named{Name: name, Namespace: "default"},
		Labels: map[string]string{wellknown.GatewayNameLabel: gw},
	}
	if subset != "" {
		pod.Annotations = map[string]string{bootstrappatch.ValidationAnnotation: subset}
	}
	return pod
}

func TestGatewayValidationBootstraps(t *testing.T) {
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
	summary := func() *validationSummary {
		gw := index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: "gw"}.ResourceName())
		if gw == nil {
			return nil
		}
		return summarize(gw.ValidationBootstraps)
	}

	// Without validation data or pods, every proxy runs Envoy's defaults.
	g.Consistently(summary).Should(BeNil(), "a Gateway without patches validates against Envoy's defaults")

	// A ConfigMap without the Gateway as owner, and pods of another Gateway, are not
	// the Gateway's.
	unowned := bootstrapConfigMap(t, "gw", raisedSubset, 8082, 9000)
	unowned.OwnerReferences = nil
	configMaps.UpdateObject(unowned)
	pods.UpdateObject(proxyPod("other-a", "other", raisedSubset))
	g.Consistently(summary).Should(BeNil(), "objects of no Gateway or another Gateway should not apply")

	// The Gateway's ConfigMap gives the current subset and the reserved ports.
	configMaps.UpdateObject(bootstrapConfigMap(t, "gw", raisedSubset, 8082, 9000))
	g.Eventually(summary).Should(Equal(&validationSummary{
		Subsets:       []ir.ValidationBootstrap{{Subset: raisedSubset}},
		ReservedPorts: []uint32{8082, 9000},
	}))

	// A pod started before the patches, without the annotation, adds Envoy's defaults,
	// named for the first such pod. An exited pod receives no configuration and is
	// left out.
	pods.UpdateObject(proxyPod("gw-b", "gw", ""))
	pods.UpdateObject(proxyPod("gw-a", "gw", ""))
	exited := proxyPod("gw-c", "gw", loweredSubset)
	exited.Terminal = true
	pods.UpdateObject(exited)
	g.Eventually(summary).Should(Equal(&validationSummary{
		Subsets:       []ir.ValidationBootstrap{{Subset: raisedSubset}, {Pod: "gw-a"}},
		ReservedPorts: []uint32{8082, 9000},
	}))

	// Once every pod has rolled to the current subset, only it remains.
	pods.UpdateObject(proxyPod("gw-a", "gw", raisedSubset))
	pods.UpdateObject(proxyPod("gw-b", "gw", raisedSubset))
	g.Eventually(summary).Should(Equal(&validationSummary{
		Subsets:       []ir.ValidationBootstrap{{Subset: raisedSubset}},
		ReservedPorts: []uint32{8082, 9000},
	}))

	// When the patches are removed, the ConfigMap loses the key, and the pods still
	// running the raised limit keep it in validation until they roll.
	cm := bootstrapConfigMap(t, "gw", bootstrappatch.EmptyValidationSubset)
	delete(cm.Data, bootstrappatch.ValidationKey)
	configMaps.UpdateObject(cm)
	g.Eventually(summary).Should(Equal(&validationSummary{
		Subsets: []ir.ValidationBootstrap{{}, {Subset: raisedSubset, Pod: "gw-a"}},
	}))

	// The parsed subsets are kept for validation.
	gw := index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: "gw"}.ResourceName())
	g.Expect(gw.ValidationBootstraps.Subsets[0].Bootstrap).To(BeNil(), "Envoy's defaults need no subset")
	g.Expect(gw.ValidationBootstraps.Subsets[1].Bootstrap.GetLayeredRuntime().GetLayers()).To(HaveLen(1))
}

func TestBuildValidationBootstraps(t *testing.T) {
	gw := types.NamespacedName{Namespace: "default", Name: "gw"}

	// A subset that sets nothing is Envoy's defaults.
	assert.Equal(t, ir.ValidationBootstrap{Pod: "gw-a"}, parseValidationSubset(gw, bootstrappatch.EmptyValidationSubset, "gw-a"))

	// A subset that does not parse is treated as Envoy's defaults.
	assert.Equal(t, ir.ValidationBootstrap{Pod: "gw-a"}, parseValidationSubset(gw, `{"layered_runtim":{}}`, "gw-a"))

	// Validation data that does not decode leaves Envoy's defaults and the static
	// reserved ports.
	broken := bootstrapConfigMap(t, "gw", raisedSubset, 9000)
	broken.Data[bootstrappatch.ValidationKey] = "not json"
	current, ports := currentValidationData(gw, []*corev1.ConfigMap{broken})
	assert.Equal(t, ir.ValidationBootstrap{}, current)
	assert.Nil(t, ports)

	// Only the static reserved ports and Envoy's defaults need no validation bootstraps.
	assert.Nil(t, buildValidationBootstraps(ir.ValidationBootstrap{}, nil, []ir.ValidationBootstrap{{Pod: "gw-a"}}))

	// Reserved ports alone still apply.
	assert.Equal(t, &ir.GatewayValidationBootstraps{ReservedPorts: []uint32{9000}},
		buildValidationBootstraps(ir.ValidationBootstrap{}, []uint32{9000}, nil))
}

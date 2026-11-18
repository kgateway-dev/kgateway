package krtcollections

import (
	"cmp"
	"slices"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// gatewayValidationBootstraps is a Gateway's validation bootstraps, keyed by the
// Gateway.
type gatewayValidationBootstraps struct {
	key        string
	bootstraps *ir.GatewayValidationBootstraps
}

func (g gatewayValidationBootstraps) ResourceName() string {
	return g.key
}

// Equals compares the encoded subsets rather than the ConfigMaps and pods they come
// from, so a pod becoming Ready, or a change to another key of the ConfigMap, stops
// here.
func (g gatewayValidationBootstraps) Equals(in gatewayValidationBootstraps) bool {
	return g.key == in.key && g.bootstraps.Equals(in.bootstraps)
}

// newGatewayValidationBootstraps builds each Gateway's validation bootstraps: the
// current subset and reserved ports from the ValidationKey of the generated
// bootstrap ConfigMap the Gateway owns, and the subsets on the Gateway's proxy pods.
func newGatewayValidationBootstraps(
	gateways krt.Collection[gatewaySpec],
	configMaps krt.Collection[*corev1.ConfigMap],
	pods krt.Collection[WrappedPod],
	krtOpts krtutil.KrtOptions,
) krt.Collection[gatewayValidationBootstraps] {
	// Index the ConfigMaps that hold validation data by the Gateway that owns them.
	configMapsByGateway := krtpkg.UnnamedIndex(configMaps, func(cm *corev1.ConfigMap) []types.NamespacedName {
		if _, ok := cm.Data[bootstrappatch.ValidationKey]; !ok {
			return nil
		}
		var out []types.NamespacedName
		for _, ref := range cm.OwnerReferences {
			gv, err := schema.ParseGroupVersion(ref.APIVersion)
			if err == nil && gv.Group == wellknown.GatewayGroup && ref.Kind == wellknown.GatewayKind {
				out = append(out, types.NamespacedName{Namespace: cm.Namespace, Name: ref.Name})
			}
		}
		return out
	})

	// Index pods by the Gateway name label the proxy Deployment sets, which holds the
	// Gateway's name shortened to fit a label value.
	podsByGateway := krtpkg.UnnamedIndex(pods, func(pod WrappedPod) []types.NamespacedName {
		name, ok := pod.Labels[wellknown.GatewayNameLabel]
		if !ok {
			return nil
		}
		return []types.NamespacedName{{Namespace: pod.Namespace, Name: name}}
	})

	return krt.NewCollection(gateways, func(kctx krt.HandlerContext, spec gatewaySpec) *gatewayValidationBootstraps {
		gw := types.NamespacedName{Namespace: spec.gw.Namespace, Name: spec.gw.Name}

		// Read the current subset and reserved ports.
		configMaps := krt.Fetch(kctx, configMaps, krt.FilterIndex(configMapsByGateway, gw))
		current, reservedPorts := currentValidationData(gw, configMaps)

		// Read the subset each running pod started with.
		podKey := types.NamespacedName{Namespace: gw.Namespace, Name: kubeutils.SafeGatewayLabelValue(gw.Name)}
		running := runningValidationSubsets(gw, krt.Fetch(kctx, pods, krt.FilterIndex(podsByGateway, podKey)))

		return &gatewayValidationBootstraps{
			key:        krt.GetKey(spec.gw),
			bootstraps: buildValidationBootstraps(current, reservedPorts, running),
		}
	}, krtOpts.ToOptions("GatewayValidationBootstraps")...)
}

// currentValidationData returns the subset and reserved ports in the ValidationKey of
// the Gateway's generated bootstrap ConfigMap. Without the key, Envoy's defaults and
// the static reserved ports apply.
func currentValidationData(gw types.NamespacedName, configMaps []*corev1.ConfigMap) (ir.ValidationBootstrap, []uint32) {
	if len(configMaps) == 0 {
		return ir.ValidationBootstrap{}, nil
	}

	// The deployer writes one generated ConfigMap per Gateway; pick by name should
	// another ConfigMap the Gateway owns have the key.
	cm := slices.MinFunc(configMaps, func(a, b *corev1.ConfigMap) int { return cmp.Compare(a.Name, b.Name) })
	data, err := bootstrappatch.DecodeValidationData(cm.Data[bootstrappatch.ValidationKey])
	if err != nil {
		logger.Error("ignoring the bootstrap validation data; validating the Gateway's routes against Envoy's defaults",
			"gateway", gw, "configmap", cm.Name, "error", err)
		return ir.ValidationBootstrap{}, nil
	}
	return parseValidationSubset(gw, string(data.Bootstrap), ""), data.ReservedPorts
}

// runningValidationSubsets returns each distinct subset the Gateway's running pods
// carry, naming the first pod by name that carries it. A pod without the annotation
// started with Envoy's defaults.
func runningValidationSubsets(gw types.NamespacedName, pods []WrappedPod) []ir.ValidationBootstrap {
	slices.SortFunc(pods, func(a, b WrappedPod) int { return cmp.Compare(a.Name, b.Name) })
	var out []ir.ValidationBootstrap
	for _, pod := range pods {
		// A pod that has exited receives no configuration.
		if pod.Terminal {
			continue
		}

		// Keep the first pod that carries each subset.
		v := parseValidationSubset(gw, pod.Annotations[bootstrappatch.ValidationAnnotation], pod.Name)
		if !slices.ContainsFunc(out, func(o ir.ValidationBootstrap) bool { return o.Subset == v.Subset }) {
			out = append(out, v)
		}
	}
	return out
}

// buildValidationBootstraps returns the bootstraps to validate under: the current
// subset first, then each other subset a running pod carries. It returns nil when
// every bootstrap is Envoy's defaults and listens only on the static reserved ports.
func buildValidationBootstraps(current ir.ValidationBootstrap, reservedPorts []uint32, running []ir.ValidationBootstrap) *ir.GatewayValidationBootstraps {
	// Validate under each distinct subset once.
	subsets := []ir.ValidationBootstrap{current}
	for _, r := range running {
		if r.Subset != current.Subset {
			subsets = append(subsets, r)
		}
	}

	// With only Envoy's defaults to validate under, validation needs no subset.
	if len(subsets) == 1 && current.Subset == "" {
		subsets = nil
	}
	if len(subsets) == 0 && len(reservedPorts) == 0 {
		return nil
	}
	return &ir.GatewayValidationBootstraps{Subsets: subsets, ReservedPorts: reservedPorts}
}

// parseValidationSubset parses an encoded subset that pod carries, or the current one
// when pod is empty. A subset that does not parse is treated as Envoy's defaults.
func parseValidationSubset(gw types.NamespacedName, subset, pod string) ir.ValidationBootstrap {
	subset = normalizeValidationSubset(subset)
	if subset == "" {
		return ir.ValidationBootstrap{Pod: pod}
	}
	bs, err := bootstrappatch.ParseValidationSubset(subset)
	if err != nil {
		logger.Error("ignoring a bootstrap validation subset that does not parse; validating against Envoy's defaults in its place",
			"gateway", gw, "pod", pod, "error", err)
		return ir.ValidationBootstrap{Pod: pod}
	}
	return ir.ValidationBootstrap{Subset: subset, Pod: pod, Bootstrap: bs}
}

// normalizeValidationSubset returns "" for a subset that sets nothing.
func normalizeValidationSubset(subset string) string {
	if subset == bootstrappatch.EmptyValidationSubset {
		return ""
	}
	return subset
}

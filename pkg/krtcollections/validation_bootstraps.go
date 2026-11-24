package krtcollections

import (
	"cmp"
	"fmt"
	"slices"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// GatewayValidation is what the translator validates a Gateway's configuration
// against, from the ValidationKey of the Gateway's generated bootstrap ConfigMap: the
// bootstrap new proxy pods start with.
type GatewayValidation struct {
	Gateway types.NamespacedName
	// Validation is nil when the Gateway's bootstrap runs Envoy's defaults and listens
	// only on the default reserved ports.
	Validation *ir.GatewayValidationBootstraps
}

func (g GatewayValidation) ResourceName() string {
	return g.Gateway.String()
}

// Equals compares the encoded subset and ports rather than the ConfigMap they come
// from, so a change to another key of the ConfigMap stops here.
func (g GatewayValidation) Equals(in GatewayValidation) bool {
	return g.Gateway == in.Gateway && g.Validation.Equals(in.Validation)
}

// GatewayRunningBootstraps is what a Gateway's proxy pods that have not exited say
// about the bootstraps they run.
type GatewayRunningBootstraps struct {
	Gateway types.NamespacedName
	// Revisions are the distinct bootstrapoverlay.ChecksumAnnotation values of the
	// Gateway's running pods, sorted.
	Revisions []string
}

func (g GatewayRunningBootstraps) ResourceName() string {
	return g.Gateway.String()
}

// Equals compares the revisions rather than the pods they come from, so a pod
// becoming Ready stops here.
func (g GatewayRunningBootstraps) Equals(in GatewayRunningBootstraps) bool {
	return g.Gateway == in.Gateway && slices.Equal(g.Revisions, in.Revisions)
}

// newGatewayValidation builds each Gateway's validation subset and reserved ports from
// the ValidationKey of the generated bootstrap ConfigMap the Gateway owns.
func newGatewayValidation(
	gateways krt.Collection[gatewaySpec],
	configMaps krt.Collection[*corev1.ConfigMap],
	krtOpts krtutil.KrtOptions,
) krt.Collection[GatewayValidation] {
	// Index the ConfigMaps that hold validation data by the Gateway that owns them.
	configMapsByGateway := krtpkg.UnnamedIndex(configMaps, func(cm *corev1.ConfigMap) []types.NamespacedName {
		if _, ok := cm.Data[bootstrapoverlay.ValidationKey]; !ok {
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

	return krt.NewCollection(gateways, func(kctx krt.HandlerContext, spec gatewaySpec) *GatewayValidation {
		gw := types.NamespacedName{Namespace: spec.gw.Namespace, Name: spec.gw.Name}
		configMaps := krt.Fetch(kctx, configMaps, krt.FilterIndex(configMapsByGateway, gw))
		return &GatewayValidation{Gateway: gw, Validation: currentValidationData(gw, configMaps)}
	}, krtOpts.ToOptions("GatewayValidation")...)
}

// newGatewayRunningBootstraps builds each Gateway's running bootstrap revisions from
// the checksum annotations of its proxy pods.
func newGatewayRunningBootstraps(
	gateways krt.Collection[gatewaySpec],
	pods krt.Collection[WrappedPod],
	krtOpts krtutil.KrtOptions,
) krt.Collection[GatewayRunningBootstraps] {
	// Index pods by the Gateway name label the proxy Deployment sets, which holds the
	// Gateway's name shortened to fit a label value.
	podsByGateway := krtpkg.UnnamedIndex(pods, func(pod WrappedPod) []types.NamespacedName {
		name, ok := pod.Labels[wellknown.GatewayNameLabel]
		if !ok {
			return nil
		}
		return []types.NamespacedName{{Namespace: pod.Namespace, Name: name}}
	})

	return krt.NewCollection(gateways, func(kctx krt.HandlerContext, spec gatewaySpec) *GatewayRunningBootstraps {
		gw := types.NamespacedName{Namespace: spec.gw.Namespace, Name: spec.gw.Name}
		podKey := types.NamespacedName{Namespace: gw.Namespace, Name: kubeutils.SafeGatewayLabelValue(gw.Name)}
		pods := krt.Fetch(kctx, pods, krt.FilterIndex(podsByGateway, podKey))
		return &GatewayRunningBootstraps{Gateway: gw, Revisions: runningRevisions(pods)}
	}, krtOpts.ToOptions("GatewayRunningBootstraps")...)
}

// currentValidationData returns the subset and reserved ports in the ValidationKey of
// the Gateway's generated bootstrap ConfigMap, or nil without the key, when Envoy's
// defaults and the default reserved ports apply.
func currentValidationData(gw types.NamespacedName, configMaps []*corev1.ConfigMap) *ir.GatewayValidationBootstraps {
	if len(configMaps) == 0 {
		return nil
	}

	// The deployer writes one generated ConfigMap per Gateway; pick by name should
	// another ConfigMap the Gateway owns have the key.
	cm := slices.MinFunc(configMaps, func(a, b *corev1.ConfigMap) int { return cmp.Compare(a.Name, b.Name) })
	data, err := bootstrapoverlay.DecodeValidationData(cm.Data[bootstrapoverlay.ValidationKey])
	if err != nil {
		logger.Error("ignoring the bootstrap validation data; validating the Gateway's routes against Envoy's defaults",
			"gateway", gw, "configmap", cm.Name, "error", err)
		return nil
	}

	// A subset that does not parse leaves Envoy's defaults, and the ports still apply.
	subset, err := parseSubset(string(data.Bootstrap))
	if err != nil {
		logger.Error("ignoring a bootstrap validation subset that does not parse; validating against Envoy's defaults in its place",
			"gateway", gw, "configmap", cm.Name, "error", err)
	}
	if subset.Subset == "" && len(data.ReservedPorts) == 0 {
		return nil
	}
	return &ir.GatewayValidationBootstraps{Subset: subset, ReservedPorts: data.ReservedPorts}
}

// runningRevisions returns the distinct checksums of the pods that have not exited,
// sorted. A pod that has exited restarts no container, so it needs no revision.
func runningRevisions(pods []WrappedPod) []string {
	var out []string
	for _, pod := range pods {
		if checksum := pod.Annotations[bootstrapoverlay.ChecksumAnnotation]; !pod.Terminal && checksum != "" {
			out = append(out, checksum)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// podBootstrapFromAnnotations returns what LocalityPod.Bootstrap holds for a pod with
// these annotations: nil without the validation annotation, otherwise the subset and
// ports of the bootstrap the pod started with. An annotation that does not decode
// gives Envoy's defaults and the default reserved ports; a subset that does not parse
// gives Envoy's defaults with the annotation's ports.
func podBootstrapFromAnnotations(annotations map[string]string) *ir.PodBootstrap {
	// A pod without the annotation is left to defaultPodBootstrap.
	annotation, ok := annotations[bootstrapoverlay.ValidationAnnotation]
	if !ok {
		return nil
	}

	// An annotation that does not decode is ignored as a whole.
	data, err := bootstrapoverlay.DecodeValidationData(annotation)
	if err != nil {
		logger.Error("ignoring a pod's bootstrap validation annotation; comparing the pod as Envoy's defaults", "error", err)
		return defaultPodBootstrap()
	}

	// A subset that does not parse leaves Envoy's defaults.
	subset, err := parseSubset(string(data.Bootstrap))
	if err != nil {
		logger.Error("ignoring a pod's bootstrap validation subset; comparing the pod as Envoy's defaults", "error", err)
	}
	return &ir.PodBootstrap{Subset: subset.Subset, Ports: data.ReservedPorts}
}

// defaultPodBootstrap is the bootstrap of a pod without the validation annotation:
// Envoy's defaults, listening on the default reserved ports.
func defaultPodBootstrap() *ir.PodBootstrap {
	return &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()}
}

// parseSubset normalizes and parses an encoded subset. A subset that does not parse
// is returned as Envoy's defaults, with the parse error.
func parseSubset(subset string) (ir.ValidationBootstrap, error) {
	// A subset that sets nothing needs no parsing.
	subset = normalizeValidationSubset(subset)
	if subset == "" {
		return ir.ValidationBootstrap{}, nil
	}

	// Parse it, falling back to Envoy's defaults.
	bs, err := bootstrapoverlay.ParseValidationSubset(subset)
	if err != nil {
		return ir.ValidationBootstrap{}, fmt.Errorf("parsing the validation subset: %w", err)
	}
	return ir.ValidationBootstrap{Subset: subset, Bootstrap: bs}, nil
}

// normalizeValidationSubset returns "" for a subset that sets nothing.
func normalizeValidationSubset(subset string) string {
	if subset == bootstrapoverlay.EmptyValidationSubset {
		return ""
	}
	return subset
}

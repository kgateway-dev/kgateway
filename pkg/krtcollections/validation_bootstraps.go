package krtcollections

import (
	"cmp"
	"fmt"
	"slices"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// GatewayRunningBootstraps describes the bootstraps a Gateway's proxies run, from the
// ValidationKey of the Gateway's generated bootstrap ConfigMap and the annotations of
// its proxy pods that have not exited.
type GatewayRunningBootstraps struct {
	Gateway types.NamespacedName
	// Validation is what the translator validates the Gateway's configuration
	// against. It is nil when every proxy runs Envoy's defaults and listens only on
	// the default reserved ports.
	Validation *ir.GatewayValidationBootstraps
	// PodPorts are the ports the bootstraps of the Gateway's running pods listen on,
	// sorted. A pod without the bootstrappatch.ValidationAnnotation listens on the
	// default reserved ports.
	PodPorts []uint32
	// Revisions are the distinct bootstrappatch.ChecksumAnnotation values of the
	// Gateway's running pods, sorted.
	Revisions []string
}

func (g GatewayRunningBootstraps) ResourceName() string {
	return g.Gateway.String()
}

// Equals compares the encoded subsets rather than the ConfigMaps and pods they come
// from, so a pod becoming Ready, or a change to another key of the ConfigMap, stops
// here.
func (g GatewayRunningBootstraps) Equals(in GatewayRunningBootstraps) bool {
	return g.Gateway == in.Gateway &&
		g.Validation.Equals(in.Validation) &&
		slices.Equal(g.PodPorts, in.PodPorts) &&
		slices.Equal(g.Revisions, in.Revisions)
}

// newGatewayRunningBootstraps builds each Gateway's running bootstraps: the current
// subset, reserved ports and static cluster names from the ValidationKey of the
// generated bootstrap ConfigMap the Gateway owns, and the subsets, ports and checksums
// on the Gateway's proxy pods.
func newGatewayRunningBootstraps(
	gateways krt.Collection[gatewaySpec],
	configMaps krt.Collection[*corev1.ConfigMap],
	pods krt.Collection[WrappedPod],
	krtOpts krtutil.KrtOptions,
) krt.Collection[GatewayRunningBootstraps] {
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

	return krt.NewCollection(gateways, func(kctx krt.HandlerContext, spec gatewaySpec) *GatewayRunningBootstraps {
		gw := types.NamespacedName{Namespace: spec.gw.Namespace, Name: spec.gw.Name}

		// Read the current subset, reserved ports and static cluster names.
		configMaps := krt.Fetch(kctx, configMaps, krt.FilterIndex(configMapsByGateway, gw))
		current := currentValidationData(gw, configMaps)

		// Read the bootstrap each running pod started with.
		podKey := types.NamespacedName{Namespace: gw.Namespace, Name: kubeutils.SafeGatewayLabelValue(gw.Name)}
		running := runningBootstraps(gw, krt.Fetch(kctx, pods, krt.FilterIndex(podsByGateway, podKey)))

		return &GatewayRunningBootstraps{
			Gateway:    gw,
			Validation: buildValidationBootstraps(current, running),
			PodPorts:   running.ports(),
			Revisions:  running.revisions(),
		}
	}, krtOpts.ToOptions("GatewayRunningBootstraps")...)
}

// currentValidation is what the ValidationKey of a Gateway's generated bootstrap
// ConfigMap says about its current bootstrap.
type currentValidation struct {
	subset         ir.ValidationBootstrap
	reservedPorts  []uint32
	staticClusters []string
}

// effectivePorts returns the ports the current bootstrap listens on: the reserved
// ports, or the default ones without validation data.
func (c currentValidation) effectivePorts() []uint32 {
	if len(c.reservedPorts) == 0 {
		return validate.DefaultReservedPorts()
	}
	return c.reservedPorts
}

// currentValidationData returns the subset, reserved ports and static cluster names in
// the ValidationKey of the Gateway's generated bootstrap ConfigMap. Without the key,
// Envoy's defaults and the default reserved ports apply, and there are no static
// clusters to check.
func currentValidationData(gw types.NamespacedName, configMaps []*corev1.ConfigMap) currentValidation {
	if len(configMaps) == 0 {
		return currentValidation{}
	}

	// The deployer writes one generated ConfigMap per Gateway; pick by name should
	// another ConfigMap the Gateway owns have the key.
	cm := slices.MinFunc(configMaps, func(a, b *corev1.ConfigMap) int { return cmp.Compare(a.Name, b.Name) })
	data, err := bootstrappatch.DecodeValidationData(cm.Data[bootstrappatch.ValidationKey])
	if err != nil {
		logger.Error("ignoring the bootstrap validation data; validating the Gateway's routes against Envoy's defaults",
			"gateway", gw, "configmap", cm.Name, "error", err)
		return currentValidation{}
	}
	return currentValidation{
		subset:         parseValidationSubset(gw, string(data.Bootstrap), ""),
		reservedPorts:  data.ReservedPorts,
		staticClusters: data.StaticClusters,
	}
}

// podBootstrap is what a running pod's annotations say about the bootstrap it started
// with.
type podBootstrap struct {
	subset   ir.ValidationBootstrap
	ports    []uint32
	checksum string
}

// podBootstraps are the bootstraps of a Gateway's running pods, sorted by pod name.
type podBootstraps []podBootstrap

// runningBootstraps returns the bootstrap of each of the Gateway's pods that has not
// exited, sorted by pod name. A pod without the validation annotation started with
// Envoy's defaults and listens on the default reserved ports.
func runningBootstraps(gw types.NamespacedName, pods []WrappedPod) podBootstraps {
	slices.SortFunc(pods, func(a, b WrappedPod) int { return cmp.Compare(a.Name, b.Name) })
	var out podBootstraps
	for _, pod := range pods {
		// A pod that has exited receives no configuration and restarts no container.
		if pod.Terminal {
			continue
		}

		// Read the pod's subset and ports, or the defaults without the annotation.
		subset, ports, err := readPodBootstrap(pod.Annotations)
		if err != nil {
			logger.Error("ignoring a pod's bootstrap validation annotation; validating against Envoy's defaults in its place",
				"gateway", gw, "pod", pod.Name, "error", err)
		}
		subset.Pod = pod.Name
		out = append(out, podBootstrap{
			subset:   subset,
			ports:    ports,
			checksum: pod.Annotations[bootstrappatch.ChecksumAnnotation],
		})
	}
	return out
}

// readPodBootstrap returns the validation subset and ports of the bootstrap a pod
// started with, from its validation annotation, with Pod unset. A pod without the
// annotation, or whose annotation does not decode, started with Envoy's defaults and
// listens on the default reserved ports. A subset that does not parse is Envoy's
// defaults, and the annotation's ports still apply. err reports what was ignored.
func readPodBootstrap(annotations map[string]string) (ir.ValidationBootstrap, []uint32, error) {
	// Without the annotation, the pod runs Envoy's defaults.
	annotation, ok := annotations[bootstrappatch.ValidationAnnotation]
	if !ok {
		return ir.ValidationBootstrap{}, validate.DefaultReservedPorts(), nil
	}

	// An annotation that does not decode is ignored as a whole.
	data, err := bootstrappatch.DecodeValidationData(annotation)
	if err != nil {
		return ir.ValidationBootstrap{}, validate.DefaultReservedPorts(), err
	}

	// A subset that does not parse leaves Envoy's defaults.
	subset, err := parseSubset(string(data.Bootstrap))
	return subset, data.ReservedPorts, err
}

// podBootstrapFromAnnotations returns what LocalityPod.Bootstrap holds for a pod with
// these annotations: nil without the validation annotation, otherwise the subset and
// ports runningBootstraps reads for it.
func podBootstrapFromAnnotations(annotations map[string]string) *ir.PodBootstrap {
	// A pod without the annotation is left to defaultPodBootstrap.
	if _, ok := annotations[bootstrappatch.ValidationAnnotation]; !ok {
		return nil
	}

	// Read it as runningBootstraps does; runningBootstraps logs what is ignored.
	subset, ports, _ := readPodBootstrap(annotations)
	return &ir.PodBootstrap{Subset: subset.Subset, Ports: ports}
}

// defaultPodBootstrap is the bootstrap of a pod without the validation annotation:
// Envoy's defaults, listening on the default reserved ports.
func defaultPodBootstrap() *ir.PodBootstrap {
	return &ir.PodBootstrap{Ports: validate.DefaultReservedPorts()}
}

// subsets returns each distinct subset the pods carry, naming the first pod that
// carries it.
func (p podBootstraps) subsets() []ir.ValidationBootstrap {
	var out []ir.ValidationBootstrap
	for _, pb := range p {
		if !slices.ContainsFunc(out, func(o ir.ValidationBootstrap) bool { return o.Subset == pb.subset.Subset }) {
			out = append(out, pb.subset)
		}
	}
	return out
}

// ports returns the ports any of the pods listens on, sorted.
func (p podBootstraps) ports() []uint32 {
	var out []uint32
	for _, pb := range p {
		out = append(out, pb.ports...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// portsExcept returns each port a pod listens on that is not in exclude, naming the
// first pod that listens on it, sorted by port.
func (p podBootstraps) portsExcept(exclude []uint32) []ir.PodReservedPort {
	var out []ir.PodReservedPort
	for _, pb := range p {
		for _, port := range pb.ports {
			if slices.Contains(exclude, port) || slices.ContainsFunc(out, func(r ir.PodReservedPort) bool { return r.Port == port }) {
				continue
			}
			out = append(out, ir.PodReservedPort{Port: port, Pod: pb.subset.Pod})
		}
	}
	slices.SortFunc(out, func(a, b ir.PodReservedPort) int { return cmp.Compare(a.Port, b.Port) })
	return out
}

// revisions returns the distinct checksums the pods carry, sorted.
func (p podBootstraps) revisions() []string {
	var out []string
	for _, pb := range p {
		if pb.checksum != "" {
			out = append(out, pb.checksum)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// buildValidationBootstraps returns the bootstraps to validate under: the current
// subset first, then each other subset a running pod carries, and the ports they
// listen on. It returns nil when every bootstrap is Envoy's defaults, listens only on
// the default reserved ports and has no static clusters to check.
func buildValidationBootstraps(current currentValidation, running podBootstraps) *ir.GatewayValidationBootstraps {
	// Validate under each distinct subset once.
	subsets := []ir.ValidationBootstrap{current.subset}
	for _, r := range running.subsets() {
		if r.Subset != current.subset.Subset {
			subsets = append(subsets, r)
		}
	}

	// With only Envoy's defaults to validate under, validation needs no subset.
	if len(subsets) == 1 && current.subset.Subset == "" {
		subsets = nil
	}

	// Reserve the ports running pods listen on that the current bootstrap does not.
	podPorts := running.portsExcept(current.effectivePorts())
	if len(subsets) == 0 && len(current.reservedPorts) == 0 && len(podPorts) == 0 && len(current.staticClusters) == 0 {
		return nil
	}
	return &ir.GatewayValidationBootstraps{
		Subsets:          subsets,
		ReservedPorts:    current.reservedPorts,
		PodReservedPorts: podPorts,
		StaticClusters:   current.staticClusters,
	}
}

// parseValidationSubset parses an encoded subset that pod carries, or the current one
// when pod is empty. A subset that does not parse is treated as Envoy's defaults.
func parseValidationSubset(gw types.NamespacedName, subset, pod string) ir.ValidationBootstrap {
	// Parse the subset, logging one that does not parse.
	vb, err := parseSubset(subset)
	if err != nil {
		logger.Error("ignoring a bootstrap validation subset that does not parse; validating against Envoy's defaults in its place",
			"gateway", gw, "pod", pod, "error", err)
	}

	// Name the pod that carries it.
	vb.Pod = pod
	return vb
}

// parseSubset normalizes and parses an encoded subset, with Pod unset. A subset that
// does not parse is returned as Envoy's defaults, with the parse error.
func parseSubset(subset string) (ir.ValidationBootstrap, error) {
	// A subset that sets nothing needs no parsing.
	subset = normalizeValidationSubset(subset)
	if subset == "" {
		return ir.ValidationBootstrap{}, nil
	}

	// Parse it, falling back to Envoy's defaults.
	bs, err := bootstrappatch.ParseValidationSubset(subset)
	if err != nil {
		return ir.ValidationBootstrap{}, fmt.Errorf("parsing the validation subset: %w", err)
	}
	return ir.ValidationBootstrap{Subset: subset, Bootstrap: bs}, nil
}

// normalizeValidationSubset returns "" for a subset that sets nothing.
func normalizeValidationSubset(subset string) string {
	if subset == bootstrappatch.EmptyValidationSubset {
		return ""
	}
	return subset
}

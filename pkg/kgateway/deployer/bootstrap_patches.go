package deployer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
)

const (
	// bootstrapConfigKey is the key of the Envoy bootstrap in the generated ConfigMap.
	bootstrapConfigKey = "envoy.yaml"
	// bootstrapVolumeName is the proxy Deployment's volume that mounts the generated
	// ConfigMap.
	bootstrapVolumeName = "envoy-config"
	// bootstrapChecksumAnnotation is the pod-template annotation that holds the SHA-256
	// of the bootstrap, so that a bootstrap change rolls the pods. The Helm template
	// sets it from the rendered bootstrap.
	bootstrapChecksumAnnotation = "kgateway.dev/checksum-config"
)

// BootstrapPatchError reports bootstrap patches that cannot be applied as written.
// None of the Gateway's objects are deployed, and retrying cannot succeed until the
// patches or the controller's configuration change.
type BootstrapPatchError struct {
	// Reason is the kgateway.dev/BootstrapPatchApplied reason to report.
	Reason string
	Err    error
}

func (e *BootstrapPatchError) Error() string {
	return e.Err.Error()
}

func (e *BootstrapPatchError) Unwrap() error {
	return e.Err
}

// BootstrapPatchOutcome describes bootstrap patches that were applied.
type BootstrapPatchOutcome struct {
	// ConfigMapName is the name of the generated bootstrap ConfigMap.
	ConfigMapName string
	// ManagedPaths lists each path the control plane manages that the patches set.
	ManagedPaths []bootstrappatch.ManagedPath
	// RemovedServicePorts lists the Gateway listener ports left off the Service because
	// a static listener in the patched bootstrap uses them.
	RemovedServicePorts []int32
}

// ApplyBootstrapPatches merges overlays, in order, into the bootstrap in the rendered
// bootstrap ConfigMap, and sets the Deployment's checksum annotation to match so the
// pods roll. It removes from the Service each port in listenerPorts that the patched
// bootstrap also listens on. Call it before applying object overlays, so that they can
// still change any object. It returns a *BootstrapPatchError when the patches cannot be
// applied, and ErrNoValidPorts when no Service port is left.
func ApplyBootstrapPatches(
	rendered []client.Object,
	overlays []bootstrappatch.Overlay,
	listenerPorts []int32,
	mode apisettings.BootstrapPatchManagedPathsMode,
) (*BootstrapPatchOutcome, error) {
	// Parse the generated bootstrap.
	cm := findBootstrapConfigMap(rendered)
	if cm == nil {
		return nil, errors.New("no bootstrap ConfigMap was rendered")
	}
	base, err := bootstrappatch.ParseBootstrap([]byte(cm.Data[bootstrapConfigKey]))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the generated bootstrap: %w", err)
	}

	// Merge the overlays into it.
	res, err := bootstrappatch.Apply(base, overlays, mode)
	var invalid *bootstrappatch.InvalidOverlayError
	var denied *bootstrappatch.ManagedPathsDeniedError
	switch {
	case errors.As(err, &invalid):
		return nil, &BootstrapPatchError{Reason: conditions.KgatewayReasonBootstrapInvalidOverlay, Err: err}
	case errors.As(err, &denied):
		return nil, &BootstrapPatchError{Reason: conditions.KgatewayReasonBootstrapPathsDenied, Err: err}
	case err != nil:
		return nil, err
	}

	// Write the merged bootstrap, and roll the pods when it changes.
	merged, err := bootstrappatch.Marshal(res.Bootstrap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal the patched bootstrap: %w", err)
	}
	cm.Data[bootstrapConfigKey] = string(merged)
	setBootstrapChecksum(rendered, merged)

	// Leave Gateway listener ports the bootstrap listens on off the Service. The
	// translator rejects those listeners.
	var colliding []int32
	for _, port := range bootstrappatch.ListenerPorts(res.Bootstrap) {
		if slices.Contains(listenerPorts, int32(port)) { //nolint:gosec // G115: Envoy ports fit in int32
			colliding = append(colliding, int32(port)) //nolint:gosec // G115: Envoy ports fit in int32
		}
	}
	removed, err := removeServicePorts(rendered, colliding)
	if err != nil {
		return nil, err
	}

	return &BootstrapPatchOutcome{
		ConfigMapName:       cm.Name,
		ManagedPaths:        res.ManagedPaths,
		RemovedServicePorts: removed,
	}, nil
}

// BootstrapMountedConfigMap returns the name of the ConfigMap the proxy Deployment's
// bootstrap volume mounts, or "" when there is no such volume. An object overlay can
// point the volume at another ConfigMap, in which case the proxy does not run the
// patched bootstrap.
func BootstrapMountedConfigMap(objs []client.Object) string {
	for _, obj := range objs {
		dep, ok := obj.(*appsv1.Deployment)
		if !ok {
			continue
		}
		for _, v := range dep.Spec.Template.Spec.Volumes {
			if v.Name == bootstrapVolumeName && v.ConfigMap != nil {
				return v.ConfigMap.Name
			}
		}
	}
	return ""
}

// bootstrapOverlays returns the bootstrap overlays of the resolved GatewayParameters,
// GatewayClass first, so that the Gateway's overlay wins a conflict.
func (r *resolvedKgatewayParameters) bootstrapOverlays() []bootstrappatch.Overlay {
	var out []bootstrappatch.Overlay
	for _, gwp := range []*kgateway.GatewayParameters{r.gatewayClassGWP, r.gatewayGWP} {
		if gwp == nil {
			continue
		}
		overlay := gwp.Spec.GetKube().GetEnvoyContainer().GetBootstrap().GetPatches().GetOverlay()
		if overlay == nil {
			continue
		}

		// A GatewayParameters referenced by both the GatewayClass and the Gateway applies once.
		source := types.NamespacedName{Namespace: gwp.Namespace, Name: gwp.Name}
		if slices.ContainsFunc(out, func(o bootstrappatch.Overlay) bool { return o.Source == source }) {
			continue
		}
		out = append(out, bootstrappatch.Overlay{Source: source, Raw: overlay.Value.Raw})
	}
	return out
}

// bootstrapPatchAppliedCondition reports patches that were applied, given the final
// objects after the object overlays.
func bootstrapPatchAppliedCondition(gw *gwv1.Gateway, outcome *BootstrapPatchOutcome, objs []client.Object) metav1.Condition {
	cond := metav1.Condition{
		Type:               conditions.KgatewayConditionBootstrapPatchApplied,
		ObservedGeneration: gw.Generation,
	}

	// An object overlay that mounts another ConfigMap keeps the patches off the proxy.
	if mounted := BootstrapMountedConfigMap(objs); mounted != outcome.ConfigMapName {
		cond.Status = metav1.ConditionFalse
		cond.Reason = conditions.KgatewayReasonBootstrapNotMounted
		cond.Message = fmt.Sprintf("The bootstrap patches are not in effect: a deploymentOverlay mounts ConfigMap %q in the %s volume instead of the generated ConfigMap %q.",
			mounted, bootstrapVolumeName, outcome.ConfigMapName)
		if mounted == "" {
			cond.Message = fmt.Sprintf("The bootstrap patches are not in effect: a deploymentOverlay removes the %s volume that mounts the generated ConfigMap %q.",
				bootstrapVolumeName, outcome.ConfigMapName)
		}
		return cond
	}

	// Report each managed path the patches set.
	cond.Status = metav1.ConditionTrue
	cond.Reason = conditions.KgatewayReasonBootstrapPatchApplied
	msg := []string{"The bootstrap patches are applied."}
	if len(outcome.ManagedPaths) > 0 {
		cond.Reason = conditions.KgatewayReasonBootstrapManagedPathsOverridden
		msg = append(msg, fmt.Sprintf(
			"They set paths the control plane manages, which can disconnect the proxy from the controller or break its probes: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	}

	// Report each listener port left off the Service.
	if len(outcome.RemovedServicePorts) > 0 {
		ports := make([]string, 0, len(outcome.RemovedServicePorts))
		for _, p := range outcome.RemovedServicePorts {
			ports = append(ports, fmt.Sprint(p))
		}
		msg = append(msg, fmt.Sprintf(
			"A static listener in the bootstrap uses Gateway listener port(s) %s, so those ports are left off the Service and the Gateway listeners on them are rejected.",
			strings.Join(ports, ", ")))
	}
	cond.Message = strings.Join(msg, " ")
	return cond
}

func joinManagedPaths(paths []bootstrappatch.ManagedPath) string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, p.String())
	}
	return strings.Join(out, ", ")
}

// findBootstrapConfigMap returns the rendered ConfigMap that holds the bootstrap.
func findBootstrapConfigMap(objs []client.Object) *corev1.ConfigMap {
	for _, obj := range objs {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			if _, ok := cm.Data[bootstrapConfigKey]; ok {
				return cm
			}
		}
	}
	return nil
}

// setBootstrapChecksum sets the checksum annotation on each rendered Deployment's pod
// template to the SHA-256 of bootstrap, the same value the Helm template computes.
func setBootstrapChecksum(objs []client.Object, bootstrap []byte) {
	sum := sha256.Sum256(bootstrap)
	for _, obj := range objs {
		dep, ok := obj.(*appsv1.Deployment)
		if !ok {
			continue
		}
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = map[string]string{}
		}
		dep.Spec.Template.Annotations[bootstrapChecksumAnnotation] = hex.EncodeToString(sum[:])
	}
}

// removeServicePorts removes each Service port whose target port is in ports, and
// returns the removed ports in order. It returns ErrNoValidPorts when that leaves a
// Service with no ports.
func removeServicePorts(objs []client.Object, ports []int32) ([]int32, error) {
	if len(ports) == 0 {
		return nil, nil
	}
	var removed []int32
	for _, obj := range objs {
		svc, ok := obj.(*corev1.Service)
		if !ok {
			continue
		}
		svc.Spec.Ports = slices.DeleteFunc(svc.Spec.Ports, func(p corev1.ServicePort) bool {
			if slices.Contains(ports, p.TargetPort.IntVal) {
				removed = append(removed, p.TargetPort.IntVal)
				return true
			}
			return false
		})
		if len(svc.Spec.Ports) == 0 {
			return nil, ErrNoValidPorts
		}
	}
	return removed, nil
}

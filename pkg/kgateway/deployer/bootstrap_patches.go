package deployer

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
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
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
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

	// applyBootstrapPatchesAction is the action of each bootstrap patch event.
	applyBootstrapPatchesAction = "ApplyBootstrapPatches"
	// curatedFieldOverriddenReason is the event reason for bootstrap patches that
	// replace what a GatewayParameters field generates.
	curatedFieldOverriddenReason = "CuratedFieldOverridden"
	// removedRuntimeGuardsReason is the event reason for bootstrap patches that set
	// runtime guards Envoy no longer has.
	removedRuntimeGuardsReason = "RemovedRuntimeGuards"
	// unvalidatedRuntimeLayerReason is the event reason for a disk or RTDS runtime layer
	// the bootstrap patches add, whose values route validation does not see.
	unvalidatedRuntimeLayerReason = "UnvalidatedRuntimeLayer"
)

// BootstrapPatchError reports bootstrap patches that were not applied. None of the
// Gateway's objects are deployed. Unless Retry is set, retrying cannot succeed until
// the patches, the controller's Deployment or its settings change.
type BootstrapPatchError struct {
	// Reason is the kgateway.dev/BootstrapPatchApplied reason to report.
	Reason string
	// ConfigMapName is the name of the Gateway's generated bootstrap ConfigMap. When it
	// does not exist yet, the Gateway has no proxy. It is empty when no bootstrap
	// ConfigMap was rendered.
	ConfigMapName string
	// ManagedPaths lists each path the control plane manages that the patches set.
	// It is nil when the paths are unknown: when an overlay is invalid, or the
	// controller failed before merging the patches.
	ManagedPaths []bootstrappatch.ManagedPath
	// Retry is set when applying the patches again can succeed without a change, for
	// example after Envoy was killed for memory or timed out while validating them.
	Retry bool
	Err   error
}

// Events returns the Warning events that report the failure.
func (e *BootstrapPatchError) Events() []deployer.ReportEvent {
	// Report each denied path on its own, so each one can be found.
	if e.Reason == conditions.KgatewayReasonBootstrapPathsDenied {
		out := make([]deployer.ReportEvent, 0, len(e.ManagedPaths))
		for _, p := range e.ManagedPaths {
			out = append(out, bootstrapPatchEvent(e.Reason,
				fmt.Sprintf("Applied none of the bootstrap patches: they set %s, which the control plane manages, and the controller is configured to reject such patches.", p)))
		}
		return out
	}
	return []deployer.ReportEvent{bootstrapPatchEvent(e.Reason, "Applied none of the bootstrap patches: "+e.Error())}
}

// pathsKnown reports whether ManagedPaths lists the paths the patches set.
func (e *BootstrapPatchError) pathsKnown() bool {
	return e.Reason != conditions.KgatewayReasonBootstrapInvalidOverlay &&
		e.Reason != conditions.KgatewayReasonBootstrapControllerError
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
	// ManagedPathsMode is the mode the patches were applied under.
	ManagedPathsMode apisettings.BootstrapPatchManagedPathsMode
	// ManagedPaths lists each path the control plane manages that the patches set.
	// Under PRUNE each one was dropped; otherwise each one applied.
	ManagedPaths []bootstrappatch.ManagedPath
	// CuratedOverrides lists each path the patches replace that a GatewayParameters
	// field generated.
	CuratedOverrides []bootstrappatch.CuratedOverride
	// RemovedServicePorts lists the Gateway listener ports left off the Service because
	// a static listener in the patched bootstrap uses them.
	RemovedServicePorts []int32
	// RemovedRuntimeGuards lists the runtime keys in the patched bootstrap that Envoy
	// reports as removed guards. Envoy ignores their values.
	RemovedRuntimeGuards []string
	// UnvalidatedLayers lists each disk and RTDS runtime layer the patches add. Route
	// validation uses Envoy's defaults for the values they set.
	UnvalidatedLayers []bootstrappatch.RuntimeLayer
}

// BootstrapPatchConfig is what ApplyBootstrapPatches needs besides the objects and
// the patches.
type BootstrapPatchConfig struct {
	// ListenerPorts are the Gateway's listener ports, which the Service exposes.
	ListenerPorts []int32
	// ManagedPaths decides what happens to patches that set paths the control plane
	// manages.
	ManagedPaths apisettings.BootstrapPatchManagedPathsMode
	// Validator validates the patched bootstrap with Envoy. When nil, the patched
	// bootstrap is not validated.
	Validator validator.BootstrapValidator
}

// ApplyBootstrapPatches merges overlays, in order, into the bootstrap in the rendered
// bootstrap ConfigMap, validates the result with Envoy, and sets the Deployment's
// checksum annotation to match so the pods roll. It writes the bootstrap's validation
// subset and listener ports to the ConfigMap's validation key, and the subset to the
// pod template, for the translator to validate against. It removes from the Service each
// listener port that the patched bootstrap also listens on. Call it before applying
// object overlays, so that they can still change any object. It returns ctx.Err()
// when ctx ends, ErrNoValidPorts when no Service port is left, and otherwise a
// *BootstrapPatchError when the patches are not applied.
func ApplyBootstrapPatches(
	ctx context.Context,
	rendered []client.Object,
	overlays []bootstrappatch.Overlay,
	cfg BootstrapPatchConfig,
) (*BootstrapPatchOutcome, error) {
	// Report a failure of the controller's own processing, which no change to the
	// patches fixes.
	cmName := ""
	controllerErr := func(err error) error {
		return &BootstrapPatchError{
			Reason:        conditions.KgatewayReasonBootstrapControllerError,
			ConfigMapName: cmName,
			Err:           fmt.Errorf("the controller failed to apply the bootstrap patches; this is a kgateway defect, not a problem with the patches: %w", err),
		}
	}

	// Parse the generated bootstrap.
	cm := findBootstrapConfigMap(rendered)
	if cm == nil {
		return nil, controllerErr(errors.New("no bootstrap ConfigMap was rendered"))
	}
	cmName = cm.Name
	base, err := bootstrappatch.ParseBootstrap([]byte(cm.Data[bootstrapConfigKey]))
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to parse the generated bootstrap: %w", err))
	}
	patchErr := func(reason string, paths []bootstrappatch.ManagedPath, err error) *BootstrapPatchError {
		return &BootstrapPatchError{Reason: reason, ConfigMapName: cm.Name, ManagedPaths: paths, Err: err}
	}

	// Merge the overlays into it.
	mode := cmp.Or(cfg.ManagedPaths, apisettings.BootstrapPatchManagedPathsWarn)
	res, err := bootstrappatch.Apply(base, overlays, mode)
	if invalid, ok := errors.AsType[*bootstrappatch.InvalidOverlayError](err); ok {
		return nil, patchErr(conditions.KgatewayReasonBootstrapInvalidOverlay, nil, invalid)
	}
	if denied, ok := errors.AsType[*bootstrappatch.ManagedPathsDeniedError](err); ok {
		return nil, patchErr(conditions.KgatewayReasonBootstrapPathsDenied, denied.Paths, denied)
	}
	if err != nil {
		return nil, controllerErr(err)
	}

	// Encode the subset of the merged bootstrap that the translator validates xDS
	// configuration against. It goes on the pod template, so it has a size limit.
	subset, err := bootstrappatch.EncodeValidationSubset(bootstrappatch.ValidationSubset(res.Bootstrap))
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to encode the validation subset of the patched bootstrap: %w", err))
	}
	if len(subset) > bootstrappatch.MaxValidationSubsetSize {
		return nil, patchErr(conditions.KgatewayReasonBootstrapInvalidOverlay, nil, fmt.Errorf(
			"the static runtime layers of the patched bootstrap take %d bytes as JSON, more than the limit of %d; set fewer runtime values in static layers",
			len(subset), bootstrappatch.MaxValidationSubsetSize))
	}

	// Validate the merged bootstrap as the proxy runs it. When ctx has ended, the
	// controller is stopping, and the result says nothing about the patches.
	var validation validator.BootstrapResult
	if cfg.Validator != nil {
		validation, err = cfg.Validator.ValidateBootstrap(ctx, res.Bootstrap)
		switch {
		case err == nil:
		case errors.Is(err, validator.ErrInvalidBootstrap):
			return nil, patchErr(conditions.KgatewayReasonBootstrapValidationFailed, res.ManagedPaths, err)
		case ctx.Err() != nil:
			return nil, fmt.Errorf("stopped validating the patched bootstrap: %w", ctx.Err())
		case errors.Is(err, validator.ErrBootstrapValidatorUnavailable):
			return nil, patchErr(conditions.KgatewayReasonBootstrapValidationUnavailable, res.ManagedPaths, fmt.Errorf(
				"%w; the controller retries after its Deployment or settings change", err))
		default:
			unavailable := patchErr(conditions.KgatewayReasonBootstrapValidationUnavailable, res.ManagedPaths, fmt.Errorf(
				"could not validate the patched bootstrap: %w; the controller retries", err))
			unavailable.Retry = true
			return nil, unavailable
		}
	}

	// Write the merged bootstrap, and roll the pods when it changes.
	merged, err := bootstrappatch.Marshal(res.Bootstrap)
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to marshal the patched bootstrap: %w", err))
	}
	cm.Data[bootstrapConfigKey] = string(merged)
	sum := sha256.Sum256(merged)
	setPodTemplateAnnotation(rendered, bootstrapChecksumAnnotation, hex.EncodeToString(sum[:]))

	// Write what the translator validates against: the subset, on the ConfigMap for
	// the pods to come and on the pod template for the pods that start with it, and
	// the ports the bootstrap listens on, where Gateway listeners are rejected.
	bootstrapPorts := bootstrappatch.ListenerPorts(res.Bootstrap)
	data, err := bootstrappatch.EncodeValidationData(subset, bootstrapPorts)
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to encode the bootstrap validation data: %w", err))
	}
	cm.Data[bootstrappatch.ValidationKey] = data
	setPodTemplateAnnotation(rendered, bootstrappatch.ValidationAnnotation, subset)

	// Leave Gateway listener ports the bootstrap listens on off the Service. The
	// translator rejects those listeners.
	var colliding []int32
	for _, port := range bootstrapPorts {
		if slices.Contains(cfg.ListenerPorts, int32(port)) { //nolint:gosec // G115: Envoy ports fit in int32
			colliding = append(colliding, int32(port)) //nolint:gosec // G115: Envoy ports fit in int32
		}
	}
	removed, err := removeServicePorts(rendered, colliding)
	if err != nil {
		return nil, err
	}

	return &BootstrapPatchOutcome{
		ConfigMapName:        cm.Name,
		ManagedPathsMode:     mode,
		ManagedPaths:         res.ManagedPaths,
		CuratedOverrides:     res.CuratedOverrides,
		RemovedServicePorts:  removed,
		RemovedRuntimeGuards: validation.RemovedRuntimeGuards,
		UnvalidatedLayers:    res.UnvalidatedLayers,
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

	// Report each managed path the patches set: under PRUNE it was dropped, otherwise
	// it applied.
	cond.Status = metav1.ConditionTrue
	cond.Reason = conditions.KgatewayReasonBootstrapPatchApplied
	msg := []string{"The bootstrap patches are applied."}
	switch {
	case len(outcome.ManagedPaths) == 0:
	case outcome.ManagedPathsMode == apisettings.BootstrapPatchManagedPathsPrune:
		cond.Reason = conditions.KgatewayReasonBootstrapPathsIgnored
		msg = append(msg, fmt.Sprintf(
			"These paths were dropped from them because the control plane manages them: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	default:
		cond.Reason = conditions.KgatewayReasonBootstrapManagedPathsOverridden
		msg = append(msg, fmt.Sprintf(
			"They set paths the control plane manages, which can disconnect the proxy from the controller or break its probes: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	}

	// Report each path that replaces what a GatewayParameters field generated. The
	// reason does not change: the replaced value is the user's own configuration.
	if len(outcome.CuratedOverrides) > 0 {
		overrides := make([]string, 0, len(outcome.CuratedOverrides))
		for _, o := range outcome.CuratedOverrides {
			overrides = append(overrides, o.String())
		}
		msg = append(msg, fmt.Sprintf(
			"The patches replace configuration other GatewayParameters fields generate, including values left at their defaults: %s.",
			strings.Join(overrides, "; ")))
	}

	// Report each listener port left off the Service.
	if len(outcome.RemovedServicePorts) > 0 {
		ports := make([]string, 0, len(outcome.RemovedServicePorts))
		for _, p := range outcome.RemovedServicePorts {
			ports = append(ports, strconv.Itoa(int(p)))
		}
		msg = append(msg, fmt.Sprintf(
			"A static listener in the bootstrap uses Gateway listener port(s) %s, so those ports are left off the Service and the Gateway listeners on them are rejected.",
			strings.Join(ports, ", ")))
	}

	// Report each runtime key Envoy ignores.
	if len(outcome.RemovedRuntimeGuards) > 0 {
		msg = append(msg, fmt.Sprintf(
			"Envoy no longer has the runtime guard(s) %s, so their values have no effect; remove them from the patches.",
			strings.Join(outcome.RemovedRuntimeGuards, ", ")))
	}

	// Report each runtime layer whose values route validation does not see.
	if len(outcome.UnvalidatedLayers) > 0 {
		layers := make([]string, 0, len(outcome.UnvalidatedLayers))
		for _, l := range outcome.UnvalidatedLayers {
			layers = append(layers, l.String())
		}
		msg = append(msg, fmt.Sprintf(
			"The proxy loads the values of these runtime layers while it runs, so route validation uses Envoy's defaults for the keys they set: %s.",
			strings.Join(layers, ", ")))
	}
	cond.Message = strings.Join(msg, " ")
	return cond
}

// bootstrapPatchEvents returns the Warning events that go with the
// kgateway.dev/BootstrapPatchApplied condition cond: one for each managed path the
// patches set, each GatewayParameters field they replace, the runtime guards Envoy
// ignores, and each disk or RTDS runtime layer they add.
func bootstrapPatchEvents(cond metav1.Condition, outcome *BootstrapPatchOutcome) []deployer.ReportEvent {
	// Patches that are not mounted have no other effect to report.
	if cond.Reason == conditions.KgatewayReasonBootstrapNotMounted {
		return []deployer.ReportEvent{bootstrapPatchEvent(cond.Reason, cond.Message)}
	}

	// Report each managed path the patches set.
	var out []deployer.ReportEvent
	for _, p := range outcome.ManagedPaths {
		if outcome.ManagedPathsMode == apisettings.BootstrapPatchManagedPathsPrune {
			out = append(out, bootstrapPatchEvent(conditions.KgatewayReasonBootstrapPathsIgnored,
				fmt.Sprintf("Dropped %s from the bootstrap patches because the control plane manages it.", p)))
			continue
		}
		out = append(out, bootstrapPatchEvent(conditions.KgatewayReasonBootstrapManagedPathsOverridden,
			fmt.Sprintf("Applied bootstrap patches that set %s, which the control plane manages.", p)))
	}

	// Report each GatewayParameters field the patches replace.
	for _, o := range outcome.CuratedOverrides {
		out = append(out, bootstrapPatchEvent(curatedFieldOverriddenReason,
			fmt.Sprintf("The bootstrap patches replace generated configuration: %s.", o)))
	}

	// Report the runtime keys Envoy ignores.
	if len(outcome.RemovedRuntimeGuards) > 0 {
		out = append(out, bootstrapPatchEvent(removedRuntimeGuardsReason,
			fmt.Sprintf("Envoy no longer has the runtime guard(s) %s, so their values have no effect; remove them from the bootstrap patches.",
				strings.Join(outcome.RemovedRuntimeGuards, ", "))))
	}

	// Report each runtime layer whose values route validation does not see.
	for _, l := range outcome.UnvalidatedLayers {
		out = append(out, bootstrapPatchEvent(unvalidatedRuntimeLayerReason,
			fmt.Sprintf("The bootstrap patches add %s. The proxy loads its values while it runs, so route validation uses Envoy's defaults for the keys it sets.", l)))
	}
	return out
}

func bootstrapPatchEvent(reason, note string) deployer.ReportEvent {
	return deployer.ReportEvent{
		Type:   corev1.EventTypeWarning,
		Reason: reason,
		Action: applyBootstrapPatchesAction,
		Note:   note,
	}
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

// setPodTemplateAnnotation sets an annotation on each rendered Deployment's pod
// template.
func setPodTemplateAnnotation(objs []client.Object, key, value string) {
	for _, obj := range objs {
		dep, ok := obj.(*appsv1.Deployment)
		if !ok {
			continue
		}
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = map[string]string{}
		}
		dep.Spec.Template.Annotations[key] = value
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

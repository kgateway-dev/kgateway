package deployer

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	// bootstrapConfigKey is the key of the Envoy bootstrap in the generated ConfigMap.
	// It holds the Gateway's current bootstrap. Each pod reads its own revision's key
	// instead (see bootstrapRevisionKey).
	bootstrapConfigKey = "envoy.yaml"
	// bootstrapRevisionPrefix and bootstrapRevisionSuffix go around a bootstrap's
	// checksum to make the key of its revision in the generated ConfigMap.
	bootstrapRevisionPrefix = "envoy-"
	bootstrapRevisionSuffix = ".yaml"
	// bootstrapMountPath is where the proxy container mounts the generated ConfigMap.
	bootstrapMountPath = "/etc/envoy"
	// bootstrapChecksumAnnotation is bootstrapoverlay.ChecksumAnnotation. The Helm
	// template sets it from the rendered bootstrap.
	bootstrapChecksumAnnotation = bootstrapoverlay.ChecksumAnnotation
	// inputConfigEnv is the proxy container's environment variable that names the
	// bootstrap file envoyinit reads. Without it, envoyinit reads envoy.yaml.
	inputConfigEnv = "INPUT_CONF"
	// maxConfigMapDataSize is the most data, all keys together, the API server accepts
	// in a ConfigMap.
	maxConfigMapDataSize = 1024 * 1024

	// applyBootstrapOverlaysAction is the action of each bootstrap overlay event.
	applyBootstrapOverlaysAction = "ApplyBootstrapOverlays"
	// writeBootstrapRevisionAction is the action of the event for a generated bootstrap
	// ConfigMap that would exceed the API server's size limit.
	writeBootstrapRevisionAction = "WriteBootstrapRevision"
)

// BootstrapOverlayProvider is an optional interface for a HelmValuesGenerator set with
// GatewayParameters.WithHelmValuesGeneratorOverride. It chooses the bootstrap overlays
// for a Gateway. The controller then applies them as it does without an override:
// it merges them into the rendered bootstrap, validates the result, removes the ports
// the bootstrap listens on from the Service, writes the xDS validation data, and
// reports the kgateway.dev/BootstrapOverlayApplied condition, events and metric.
//
// The merge runs before the override's PostProcessObjects, when the override
// implements deployer.ObjectPostProcessor, so the post-processor sees the merged
// bootstrap. An override that implements deployer.ObjectPostProcessor without this
// interface replaces the bootstrap overlays and the object overlays: no bootstrap
// overlays are applied, and no condition is reported. The controller still writes the
// Gateway's bootstrap as its own revision after the post-processor runs.
type BootstrapOverlayProvider interface {
	// BootstrapOverlays returns the overlays to merge into gw's bootstrap, in order; a
	// later overlay wins a conflict. fromGatewayParameters are the overlays of the
	// GatewayParameters that gw's GatewayClass and gw reference, GatewayClass first.
	// An overlay from another kind of resource sets Source.Kind, so status
	// messages and events name that resource.
	BootstrapOverlays(ctx context.Context, gw *gwv1.Gateway, fromGatewayParameters []bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error)
}

// BootstrapOverlayError reports bootstrap overlays that were not applied. None of the
// Gateway's objects are deployed. Unless Retry is set, retrying cannot succeed until
// the overlays, the controller's Deployment or its settings change.
type BootstrapOverlayError struct {
	// Reason is the kgateway.dev/BootstrapOverlayApplied reason to report.
	Reason string
	// ConfigMapName is the name of the Gateway's generated bootstrap ConfigMap. When it
	// does not exist yet, the Gateway has no proxy. It is empty when no bootstrap
	// ConfigMap was rendered.
	ConfigMapName string
	// ManagedPaths lists each path the control plane manages that the overlays set.
	// It is nil when the paths are unknown: when an overlay is invalid, or the
	// controller failed before merging the overlays.
	ManagedPaths []bootstrapoverlay.ManagedPath
	// Retry is set when applying the overlays again can succeed without a change, for
	// example after Envoy was killed for memory or timed out while validating them.
	Retry bool
	// Proxy is the proxy the deployer rendered and did not deploy, so the reconciler can
	// report a proxy image the failure holds back.
	Proxy RenderedProxy
	// MountProblem is BootstrapMountProblem of the objects the deployer rendered and did
	// not deploy, or "" when the proxy reads the generated bootstrap or the objects are
	// unknown. When it is set, the overlays could not reach the proxy.
	MountProblem string
	// ObjectsErr is the error from post-processing the rendered objects, such as an
	// object overlay that does not apply, or nil when they post-process. It is the
	// deployer's verdict on the Gateway's parameters apart from the bootstrap overlays.
	ObjectsErr error
	Err        error
}

// MountNote returns a sentence for the status that says the overlays could not reach
// the proxy and how to deploy the held-back objects, or "" when MountProblem is empty.
func (e *BootstrapOverlayError) MountNote() string {
	if e.MountProblem == "" {
		return ""
	}
	return fmt.Sprintf("The proxy does not read the generated bootstrap (%s), so the overlay has no effect on it; remove the overlay, or fix it before removing that change, to deploy the Gateway's other changes.", e.MountProblem)
}

// Events returns the Warning events that report the failure.
func (e *BootstrapOverlayError) Events() []deployer.ReportEvent {
	// Report each denied path on its own, so each one can be found.
	if e.Reason == conditions.KgatewayReasonBootstrapPathsDenied {
		out := make([]deployer.ReportEvent, 0, len(e.ManagedPaths))
		for _, p := range e.ManagedPaths {
			out = append(out, bootstrapOverlayEvent(e.Reason,
				fmt.Sprintf("Applied no bootstrap overlay: an overlay sets %s, which kgateway manages, and KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS is REJECT.", p)))
		}
		return out
	}
	return []deployer.ReportEvent{bootstrapOverlayEvent(e.Reason, "Applied no bootstrap overlay: "+e.Error())}
}

// pathsKnown reports whether ManagedPaths lists the paths the overlays set.
func (e *BootstrapOverlayError) pathsKnown() bool {
	return e.Reason != conditions.KgatewayReasonBootstrapInvalidOverlay &&
		e.Reason != conditions.KgatewayReasonBootstrapControllerError
}

func (e *BootstrapOverlayError) Error() string {
	return e.Err.Error()
}

func (e *BootstrapOverlayError) Unwrap() error {
	return e.Err
}

// BootstrapOverlayOutcome describes bootstrap overlays that were applied.
type BootstrapOverlayOutcome struct {
	ConfigMapName    string
	ManagedPathsMode apisettings.BootstrapOverlayManagedPathsMode
	// ManagedPaths lists each path the control plane manages that the overlays set.
	// Under PRUNE each one was dropped; otherwise each one applied.
	ManagedPaths []bootstrapoverlay.ManagedPath
	// BootstrapPorts are the ports the merged bootstrap listens on: the admin port
	// and the port of each static listener address, sorted.
	BootstrapPorts []uint32
	// RemovedServicePorts lists the Gateway listener ports left off the Service because
	// the merged bootstrap listens on them. ReserveServicePorts sets it.
	RemovedServicePorts []int32

	// checksum is the hex SHA-256 of the merged bootstrap, which names its revision.
	checksum string
	// bootstrap is the merged bootstrap, as written to the ConfigMap.
	bootstrap string
	// validationData is the content of the ConfigMap's validation key and the value
	// of the pod template's validation annotation.
	validationData string
}

// BootstrapOverlayConfig is what ApplyBootstrapOverlays needs besides the objects and
// the overlays.
type BootstrapOverlayConfig struct {
	// ManagedPaths decides what happens to overlays that set paths the control plane manages.
	ManagedPaths apisettings.BootstrapOverlayManagedPathsMode
	// Validator validates the merged bootstrap with Envoy. When nil, the merged
	// bootstrap is not validated.
	Validator validator.BootstrapValidator
}

// ApplyBootstrapOverlays merges overlays, in order, into the bootstrap in the rendered
// bootstrap ConfigMap, validates the result with Envoy, and sets the Deployment's
// checksum annotation to match so the pods roll. Call it before applying object
// overlays, so that they can still change any object, and then WriteBootstrapRevision
// and ReserveServicePorts once they are applied. It returns ctx.Err() when ctx ends,
// and otherwise a *BootstrapOverlayError when the overlays are not applied.
func ApplyBootstrapOverlays(
	ctx context.Context,
	rendered []client.Object,
	overlays []bootstrapoverlay.Overlay,
	cfg BootstrapOverlayConfig,
) (*BootstrapOverlayOutcome, error) {
	// Report a failure of the controller's own processing, which no change to the
	// overlays fixes.
	cmName := ""
	controllerErr := func(err error) error {
		return &BootstrapOverlayError{
			Reason:        conditions.KgatewayReasonBootstrapControllerError,
			ConfigMapName: cmName,
			Err:           fmt.Errorf("the controller failed to apply the bootstrap overlay; this is a kgateway defect, not a problem with the overlay: %w", err),
		}
	}

	// Parse the generated bootstrap.
	cm := findBootstrapConfigMap(rendered)
	if cm == nil {
		return nil, controllerErr(errors.New("no bootstrap ConfigMap was rendered"))
	}
	cmName = cm.Name
	base, err := bootstrapoverlay.ParseBootstrap([]byte(cm.Data[bootstrapConfigKey]))
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to parse the generated bootstrap: %w", err))
	}
	overlayErr := func(reason string, paths []bootstrapoverlay.ManagedPath, err error) *BootstrapOverlayError {
		return &BootstrapOverlayError{Reason: reason, ConfigMapName: cm.Name, ManagedPaths: paths, Err: err}
	}

	// Merge the overlays into it.
	mode := cmp.Or(cfg.ManagedPaths, apisettings.BootstrapOverlayManagedPathsWarn)
	res, err := bootstrapoverlay.Apply(base, overlays, mode)
	if _, ok := errors.AsType[*bootstrapoverlay.InvalidOverlayError](err); ok {
		return nil, overlayErr(conditions.KgatewayReasonBootstrapInvalidOverlay, nil, err)
	}
	if denied, ok := errors.AsType[*bootstrapoverlay.ManagedPathsDeniedError](err); ok {
		return nil, overlayErr(conditions.KgatewayReasonBootstrapPathsDenied, denied.Paths, denied)
	}
	if err != nil {
		return nil, controllerErr(err)
	}

	// Encode the subset of the merged bootstrap that the translator validates xDS
	// configuration against. It goes on the pod template, so it has a size limit.
	subset, err := bootstrapoverlay.EncodeValidationSubset(bootstrapoverlay.ValidationSubset(res.Bootstrap))
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to encode the validation subset of the merged bootstrap: %w", err))
	}
	if len(subset) > bootstrapoverlay.MaxValidationSubsetSize {
		return nil, overlayErr(conditions.KgatewayReasonBootstrapInvalidOverlay, nil, fmt.Errorf(
			"the static runtime layers of the merged bootstrap take %d bytes as JSON, more than the limit of %d; set fewer runtime values in static layers",
			len(subset), bootstrapoverlay.MaxValidationSubsetSize))
	}

	// Validate the merged bootstrap as the proxy runs it. When ctx has ended, the
	// controller is stopping, and the result says nothing about the overlays.
	if cfg.Validator != nil {
		err := cfg.Validator.ValidateBootstrap(ctx, res.Bootstrap)
		switch {
		case err == nil:
		case errors.Is(err, validator.ErrInvalidBootstrap):
			return nil, overlayErr(conditions.KgatewayReasonBootstrapValidationFailed, res.ManagedPaths, err)
		case ctx.Err() != nil:
			return nil, fmt.Errorf("stopped validating the merged bootstrap: %w", ctx.Err())
		case errors.Is(err, validator.ErrBootstrapValidatorUnavailable):
			return nil, overlayErr(conditions.KgatewayReasonBootstrapValidationUnavailable, res.ManagedPaths, fmt.Errorf(
				"%w; the controller retries after its Deployment or settings change", err))
		default:
			unavailable := overlayErr(conditions.KgatewayReasonBootstrapValidationUnavailable, res.ManagedPaths, fmt.Errorf(
				"could not validate the merged bootstrap: %w; the controller retries", err))
			unavailable.Retry = true
			return nil, unavailable
		}
	}

	// Write the merged bootstrap, and roll the pods when it changes.
	merged, err := bootstrapoverlay.Marshal(res.Bootstrap)
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to marshal the merged bootstrap: %w", err))
	}
	cm.Data[bootstrapConfigKey] = string(merged)
	sum := sha256.Sum256(merged)
	checksum := hex.EncodeToString(sum[:])
	setPodTemplateAnnotation(rendered, bootstrapChecksumAnnotation, checksum)

	// Encode what the translator validates against, which WriteBootstrapRevision writes
	// to the ConfigMap and the pod template: the subset, and the ports the bootstrap
	// listens on, where Gateway listeners are rejected.
	bootstrapPorts := bootstrapoverlay.ListenerPorts(res.Bootstrap)
	data, err := bootstrapoverlay.EncodeValidationData(subset, bootstrapPorts)
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to encode the bootstrap validation data: %w", err))
	}

	return &BootstrapOverlayOutcome{
		ConfigMapName:    cm.Name,
		ManagedPathsMode: mode,
		ManagedPaths:     res.ManagedPaths,
		BootstrapPorts:   bootstrapPorts,
		checksum:         checksum,
		bootstrap:        string(merged),
		validationData:   data,
	}, nil
}

// ReserveServicePorts leaves off the rendered Service each Gateway listener port, in
// listenerPorts, that the merged bootstrap listens on, and records the removed ports
// on outcome. The translator rejects the Gateway listeners on those ports, because the
// proxy's static listener holds the port. The default reserved ports are never on the
// Service. Call it after the object overlays, so that a serviceOverlay cannot add a
// reserved port back, and only when the proxy runs the merged bootstrap (see
// BootstrapMountProblem). It returns a *BootstrapOverlayError when that would leave the
// Service with no ports, since a Service with no ports cannot be applied.
func ReserveServicePorts(objs []client.Object, outcome *BootstrapOverlayOutcome, listenerPorts []int32) error {
	// Leave off the ports the merged bootstrap listens on.
	colliding := listenerPortsIn(listenerPorts, outcome.BootstrapPorts)
	removed, err := removeServicePorts(objs, colliding)

	// Name the colliding ports when they are all the Service has, so the overlay can be
	// fixed. The proxy keeps running its current bootstrap.
	if errors.Is(err, ErrNoValidPorts) {
		return &BootstrapOverlayError{
			Reason:        conditions.KgatewayReasonBootstrapInvalidOverlay,
			ConfigMapName: outcome.ConfigMapName,
			Err: fmt.Errorf(
				"the merged bootstrap listens on every Gateway listener port (%s), which would leave the Gateway's Service with no ports; move the bootstrap's static listener or add a Gateway listener on another port",
				joinPorts(colliding)),
		}
	}
	if err != nil {
		return err
	}
	outcome.RemovedServicePorts = removed
	return nil
}

// BootstrapRevisions is what the cluster holds of the bootstrap revisions a Gateway's
// proxies use.
type BootstrapRevisions struct {
	// Live is the Gateway's generated bootstrap ConfigMap as the cluster has it; nil
	// before it exists.
	Live *corev1.ConfigMap
	// InUse are the checksums of the bootstrap revisions in use: those of the
	// Gateway's pods that have not exited, and of the ReplicaSets of its proxy
	// Deployment with replicas above zero, which can still create a pod.
	InUse []string
	// Previous is the checksum of the revision Live is current on, or "" without one.
	// The proxy Deployment's template can still name it, and a ReplicaSet or pod
	// created from that template may not be in InUse yet, so the revision is kept in
	// the next ConfigMap written when there is room for it. A write that makes
	// another revision current ends this: the ConfigMap is then current on that one.
	Previous string
}

// currentBootstrapRevision returns the checksum of the revision cm is current on: the
// one whose key holds the same content as envoy.yaml. It returns "" when cm is nil or
// has no such key.
func currentBootstrapRevision(cm *corev1.ConfigMap) string {
	// Find the bootstrap.
	if cm == nil {
		return ""
	}
	bootstrap, ok := cm.Data[bootstrapConfigKey]
	if !ok {
		return ""
	}

	// Find the revision key that holds it.
	for _, key := range slices.Sorted(maps.Keys(cm.Data)) {
		sum, isRevision := strings.CutPrefix(key, bootstrapRevisionPrefix)
		if !isRevision {
			continue
		}
		sum, isRevision = strings.CutSuffix(sum, bootstrapRevisionSuffix)
		if isRevision && cm.Data[key] == bootstrap {
			return sum
		}
	}
	return ""
}

// BootstrapConfigMapTooLargeError reports a generated bootstrap ConfigMap that would
// exceed the API server's size limit with the revisions it has to hold. None of the
// Gateway's objects are deployed. For a Gateway with bootstrap overlays it is wrapped in
// a *BootstrapOverlayError; for a Gateway without them it is returned as is, and the
// reconciler reports it as a Warning event.
type BootstrapConfigMapTooLargeError struct {
	ConfigMapName string
	// Proxy is as on BootstrapOverlayError.
	Proxy RenderedProxy
	Err   error
}

// Events returns the Warning event that reports the failure for a Gateway without
// bootstrap overlays.
func (e *BootstrapConfigMapTooLargeError) Events() []deployer.ReportEvent {
	return []deployer.ReportEvent{{
		Type:   corev1.EventTypeWarning,
		Reason: conditions.KgatewayReasonBootstrapConfigMapTooLarge,
		Action: writeBootstrapRevisionAction,
		Note:   "Deployed none of the Gateway's objects: " + e.Error(),
	}}
}

func (e *BootstrapConfigMapTooLargeError) Error() string {
	return e.Err.Error()
}

func (e *BootstrapConfigMapTooLargeError) Unwrap() error {
	return e.Err
}

// WriteBootstrapRevision writes the merged bootstrap that ApplyBootstrapOverlays
// produced as its own revision: under the envoy-<checksum>.yaml key of the generated
// ConfigMap, which INPUT_CONF on the proxy container points at, so a pod always reads
// the bootstrap its annotations describe, even when its container restarts during a
// rollout. It also writes the validation data to the ConfigMap's validation key and the
// pod template. It keeps each earlier revision in revs.InUse that the live ConfigMap
// holds, and revs.Previous when there is room for it, and leaves out the rest, so
// applying the ConfigMap removes them. Call it
// after the object overlays, and only when the proxy mounts the generated ConfigMap
// (see BootstrapMountProblem). It returns a *BootstrapOverlayError when the ConfigMap
// would exceed the API server's size limit.
func WriteBootstrapRevision(objs []client.Object, outcome *BootstrapOverlayOutcome, revs BootstrapRevisions) error {
	// Write the validation data.
	cm := findBootstrapConfigMap(objs)
	if cm == nil {
		return &BootstrapOverlayError{
			Reason: conditions.KgatewayReasonBootstrapControllerError,
			Err:    errors.New("the controller failed to apply the bootstrap overlay; this is a kgateway defect, not a problem with the overlay: no bootstrap ConfigMap was rendered"),
		}
	}
	cm.Data[bootstrapoverlay.ValidationKey] = outcome.validationData
	setPodTemplateAnnotation(objs, bootstrapoverlay.ValidationAnnotation, outcome.validationData)

	// Write this revision and the earlier ones in use.
	return outcome.wrapTooLarge(writeBootstrapRevisions(objs, cm, outcome.checksum, outcome.bootstrap, revs, true))
}

// WriteGeneratedBootstrapRevision is WriteBootstrapRevision for a Gateway without
// bootstrap overlays: it writes the generated bootstrap as its own revision, named by
// the pod template's checksum annotation, and points INPUT_CONF at it, so adding or
// removing an overlay changes only the content of the next revision. It writes no
// validation data. It does nothing when no bootstrap ConfigMap is rendered, and
// returns a *BootstrapConfigMapTooLargeError when the ConfigMap would exceed the API
// server's size limit.
func WriteGeneratedBootstrapRevision(objs []client.Object, revs BootstrapRevisions) error {
	// Find the generated bootstrap.
	cm := findBootstrapConfigMap(objs)
	if cm == nil {
		return nil
	}

	// Name the revision by the pod template's checksum, which Helm computes from the
	// bootstrap, so the key matches the annotation the pods carry. Without one, set it.
	checksum := podTemplateAnnotation(objs, bootstrapChecksumAnnotation)
	if checksum == "" {
		sum := sha256.Sum256([]byte(cm.Data[bootstrapConfigKey]))
		checksum = hex.EncodeToString(sum[:])
		setPodTemplateAnnotation(objs, bootstrapChecksumAnnotation, checksum)
	}

	// Write this revision and the earlier ones in use.
	return writeBootstrapRevisions(objs, cm, checksum, cm.Data[bootstrapConfigKey], revs, false)
}

// KeepBootstrapRevisions is for a proxy that does not read the generated ConfigMap
// (see BootstrapMountProblem): it writes no revision for the current bootstrap and
// does not set INPUT_CONF, so envoyinit reads envoy.yaml from what the proxy mounts,
// or the file an INPUT_CONF from the GatewayParameters names.
// It keeps each earlier revision in revs.InUse that the live ConfigMap holds, so pods
// that still mount the generated ConfigMap keep reading their own, and revs.Previous
// when there is room for it. No revision becomes current, so revs.Previous is the same
// revision at each reconcile until envoy.yaml changes. outcome is nil for
// a Gateway without bootstrap overlays. It returns a *BootstrapOverlayError, or without
// overlays a *BootstrapConfigMapTooLargeError, when the ConfigMap would exceed the API
// server's size limit.
func KeepBootstrapRevisions(objs []client.Object, outcome *BootstrapOverlayOutcome, revs BootstrapRevisions) error {
	cm := findBootstrapConfigMap(objs)
	if cm == nil {
		return nil
	}
	return outcome.wrapTooLarge(writeBootstrapRevisions(objs, cm, "", cm.Data[bootstrapConfigKey], revs, outcome != nil))
}

// writeBootstrapRevisions writes bootstrap under the revision key for checksum and
// points INPUT_CONF on the proxy container at it; with an empty checksum it writes
// neither. It then copies each earlier revision in revs.InUse that the live ConfigMap
// holds, and revs.Previous when there is room for it, and checks the API server's size
// limit. A revision in use is never dropped to make room: a pod reading it would wait
// for it when its container restarts.
// hasOverlays says whether the Gateway has bootstrap overlays, for the error message.
func writeBootstrapRevisions(objs []client.Object, cm *corev1.ConfigMap, checksum, bootstrap string, revs BootstrapRevisions, hasOverlays bool) error {
	// Write this revision, and point the proxy at it.
	if checksum != "" {
		key := bootstrapRevisionKey(checksum)
		cm.Data[key] = bootstrap
		setProxyEnv(objs, inputConfigEnv, path.Join(bootstrapMountPath, key))
	}

	// Keep each earlier revision in use that the live ConfigMap holds.
	earlierCount, earlierSize := 0, 0
	for _, sum := range slices.Compact(slices.Sorted(slices.Values(revs.InUse))) {
		if sum == "" || sum == checksum || revs.Live == nil {
			continue
		}
		if prev, ok := revs.Live.Data[bootstrapRevisionKey(sum)]; ok {
			cm.Data[bootstrapRevisionKey(sum)] = prev
			earlierCount++
			earlierSize += len(prev)
		}
	}

	// Keep the revision the live ConfigMap is current on, when there is room for it.
	// Nothing is known to use it, so it does not count against the size limit.
	prevKey := bootstrapRevisionKey(revs.Previous)
	if _, written := cm.Data[prevKey]; revs.Previous != "" && !written && revs.Live != nil {
		if prev, ok := revs.Live.Data[prevKey]; ok && configMapDataSize(cm)+len(prev) <= maxConfigMapDataSize {
			cm.Data[prevKey] = prev
		}
	}

	// Check the API server's size limit.
	size := configMapDataSize(cm)
	if size <= maxConfigMapDataSize {
		return nil
	}
	msg := fmt.Sprintf("the generated ConfigMap %q would hold %d bytes, more than the %d bytes the API server allows; the new bootstrap takes %d bytes",
		cm.Name, size, maxConfigMapDataSize, len(bootstrap))
	switch {
	case earlierCount > 0:
		msg += fmt.Sprintf(", and %d earlier bootstrap revisions that proxy pods or ReplicaSets still use take %d bytes; the change applies once the rollout finishes, or once the Gateway's old ReplicaSets are scaled down or their pods deleted",
			earlierCount, earlierSize)
	case hasOverlays:
		msg += "; make the bootstrap overlay smaller"
	}
	return &BootstrapConfigMapTooLargeError{ConfigMapName: cm.Name, Err: errors.New(msg)}
}

// wrapTooLarge returns err as a *BootstrapOverlayError when it is a
// *BootstrapConfigMapTooLargeError and the Gateway has bootstrap overlays (o is not
// nil), so it is reported on the kgateway.dev/BootstrapOverlayApplied condition.
// Otherwise it returns err unchanged.
func (o *BootstrapOverlayOutcome) wrapTooLarge(err error) error {
	tooLarge, ok := errors.AsType[*BootstrapConfigMapTooLargeError](err)
	if !ok || o == nil {
		return err
	}
	return &BootstrapOverlayError{
		Reason:        conditions.KgatewayReasonBootstrapConfigMapTooLarge,
		ConfigMapName: tooLarge.ConfigMapName,
		ManagedPaths:  o.ManagedPaths,
		Err:           tooLarge,
	}
}

// BootstrapMountProblem returns why the proxy container of the Deployment in objs does
// not read the generated ConfigMap cmName at /etc/envoy, or "" when it does. An object
// overlay can point the bootstrap volume at another ConfigMap, or mount another volume
// there. The GatewayParameters env or an object overlay can set INPUT_CONF, which
// names the file envoyinit reads, to a path outside /etc/envoy. The GatewayParameters
// extraVolumeMounts or an object overlay can mount another volume inside /etc/envoy,
// over the file envoyinit reads. In each case the proxy does not run the merged
// bootstrap. Call it before the controller sets INPUT_CONF on the container.
func BootstrapMountProblem(objs []client.Object, cmName string) string {
	// Find the proxy container.
	dep := findDeployment(objs)
	if dep == nil {
		return "no proxy Deployment is rendered"
	}
	podSpec := dep.Spec.Template.Spec
	i := slices.IndexFunc(podSpec.Containers, func(c corev1.Container) bool { return c.Name == wellknown.KgatewayContainerName })
	if i < 0 {
		return fmt.Sprintf("a deploymentOverlay removes the %s container", wellknown.KgatewayContainerName)
	}
	container := podSpec.Containers[i]

	// Find what the container mounts at /etc/envoy.
	j := slices.IndexFunc(container.VolumeMounts, func(m corev1.VolumeMount) bool { return path.Clean(m.MountPath) == bootstrapMountPath })
	if j < 0 {
		return fmt.Sprintf("a deploymentOverlay removes the %s container's mount at %s", wellknown.KgatewayContainerName, bootstrapMountPath)
	}
	mount := container.VolumeMounts[j]
	k := slices.IndexFunc(podSpec.Volumes, func(v corev1.Volume) bool { return v.Name == mount.Name })

	// Check that it is the whole generated ConfigMap.
	switch {
	case k < 0 || podSpec.Volumes[k].ConfigMap == nil:
		return fmt.Sprintf("a deploymentOverlay mounts volume %q at %s, which is not the generated ConfigMap %q", mount.Name, bootstrapMountPath, cmName)
	case podSpec.Volumes[k].ConfigMap.Name != cmName:
		return fmt.Sprintf("a deploymentOverlay mounts ConfigMap %q at %s instead of the generated ConfigMap %q",
			podSpec.Volumes[k].ConfigMap.Name, bootstrapMountPath, cmName)
	case mount.SubPath != "" || mount.SubPathExpr != "":
		return fmt.Sprintf("a deploymentOverlay mounts only part of the generated ConfigMap %q at %s", cmName, bootstrapMountPath)
	case len(podSpec.Volumes[k].ConfigMap.Items) > 0:
		return fmt.Sprintf("a deploymentOverlay mounts only some keys of the generated ConfigMap %q at %s", cmName, bootstrapMountPath)
	}

	// Find the file the proxy reads when the controller sets no INPUT_CONF: envoy.yaml in
	// /etc/envoy, or the file an INPUT_CONF on the container names. The rendered
	// Deployment has no INPUT_CONF, so one here comes from the GatewayParameters env or
	// an object overlay, and the last one applies. A value that is not a literal path,
	// such as one from a valueFrom, cleans to a path outside /etc/envoy.
	bootstrapFile := path.Join(bootstrapMountPath, bootstrapConfigKey)
	for _, e := range container.Env {
		if e.Name == inputConfigEnv {
			bootstrapFile = path.Clean(e.Value)
		}
	}

	// Check that the file is inside /etc/envoy.
	if !strings.HasPrefix(bootstrapFile, bootstrapMountPath+"/") {
		return fmt.Sprintf("the %s container's env sets %s to a path outside %s, so the proxy reads its bootstrap from that file and not from the generated ConfigMap %q",
			wellknown.KgatewayContainerName, inputConfigEnv, bootstrapMountPath, cmName)
	}

	// Check that no other volume holds the file: one mounted inside /etc/envoy at the
	// file's path, or at a directory above it. With any other mount inside /etc/envoy,
	// the file is in the generated ConfigMap, and the controller's INPUT_CONF replaces
	// the container's.
	for n, m := range container.VolumeMounts {
		mountPath := path.Clean(m.MountPath)
		inside := strings.HasPrefix(mountPath, bootstrapMountPath+"/")
		holdsFile := bootstrapFile == mountPath || strings.HasPrefix(bootstrapFile, mountPath+"/")
		if n != j && inside && holdsFile {
			return fmt.Sprintf("the %s container mounts volume %q at %s, so the proxy reads its bootstrap file %s from that volume and not from the generated ConfigMap %q",
				wellknown.KgatewayContainerName, m.Name, mountPath, bootstrapFile, cmName)
		}
	}
	return ""
}

// bootstrapRevisionKey returns the generated ConfigMap's key for the bootstrap with
// checksum sum.
func bootstrapRevisionKey(sum string) string {
	return bootstrapRevisionPrefix + sum + bootstrapRevisionSuffix
}

// configMapDataSize returns the size the API server limits: the values of all keys together.
func configMapDataSize(cm *corev1.ConfigMap) int {
	size := 0
	for _, v := range cm.Data {
		size += len(v)
	}
	for _, v := range cm.BinaryData {
		size += len(v)
	}
	return size
}

// listenerPortsIn returns each of listenerPorts that is in ports, in order.
func listenerPortsIn(listenerPorts []int32, ports []uint32) []int32 {
	var out []int32
	for _, p := range listenerPorts {
		if slices.Contains(ports, uint32(p)) { //nolint:gosec // G115: listener ports are positive
			out = append(out, p)
		}
	}
	return out
}

// bootstrapOverlays returns the bootstrap overlays of the resolved GatewayParameters,
// GatewayClass first, so that the Gateway's overlay wins a conflict.
func (r *resolvedKgatewayParameters) bootstrapOverlays() []bootstrapoverlay.Overlay {
	var out []bootstrapoverlay.Overlay
	for _, gwp := range []*kgateway.GatewayParameters{r.gatewayClassGWP, r.gatewayGWP} {
		if gwp == nil {
			continue
		}
		overlay := gwp.Spec.GetKube().GetEnvoyContainer().GetBootstrap().GetOverlay()
		if overlay == nil {
			continue
		}

		// A GatewayParameters referenced by both the GatewayClass and the Gateway applies once.
		source := bootstrapoverlay.Source{Namespace: gwp.Namespace, Name: gwp.Name}
		if slices.ContainsFunc(out, func(o bootstrapoverlay.Overlay) bool { return o.Source == source }) {
			continue
		}
		out = append(out, bootstrapoverlay.Overlay{Source: source, Raw: overlay.Value.Raw})
	}
	return out
}

// bootstrapOverlayAppliedCondition reports overlays that were applied. mountProblem is
// BootstrapMountProblem of the final objects, after the object overlays.
func bootstrapOverlayAppliedCondition(gw *gwv1.Gateway, outcome *BootstrapOverlayOutcome, mountProblem string) metav1.Condition {
	cond := metav1.Condition{
		Type:               conditions.KgatewayConditionBootstrapOverlayApplied,
		ObservedGeneration: gw.Generation,
	}

	// An object overlay that mounts something else at /etc/envoy keeps the bootstrap
	// overlays off the proxy.
	if mountProblem != "" {
		cond.Status = metav1.ConditionFalse
		cond.Reason = conditions.KgatewayReasonBootstrapNotMounted
		cond.Message = fmt.Sprintf("The bootstrap overlay is not in effect: %s. Routes are validated against Envoy's defaults, not the overlay.", mountProblem)
		return cond
	}

	// Report each managed path the overlays set: under PRUNE it was dropped, otherwise
	// it applied.
	cond.Status = metav1.ConditionTrue
	cond.Reason = conditions.KgatewayReasonBootstrapOverlayApplied
	msg := []string{"The bootstrap overlay is applied."}
	switch {
	case len(outcome.ManagedPaths) == 0:
	case outcome.ManagedPathsMode == apisettings.BootstrapOverlayManagedPathsPrune:
		cond.Reason = conditions.KgatewayReasonBootstrapPathsIgnored
		msg = append(msg, fmt.Sprintf(
			"These paths were dropped from it because kgateway manages them: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	default:
		cond.Reason = conditions.KgatewayReasonBootstrapManagedPathsOverridden
		msg = append(msg, fmt.Sprintf(
			"It sets paths kgateway manages, which can disconnect the proxy from the controller or break its probes: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	}

	// Report each listener port left off the Service.
	if len(outcome.RemovedServicePorts) > 0 {
		msg = append(msg, fmt.Sprintf(
			"A static listener in the bootstrap uses Gateway listener port(s) %s, so those ports are left off the Service and the Gateway listeners on them are rejected.",
			joinPorts(outcome.RemovedServicePorts)))
	}
	cond.Message = strings.Join(msg, " ")
	return cond
}

// bootstrapOverlayEvents returns the Warning events that go with the
// kgateway.dev/BootstrapOverlayApplied condition cond: one for each managed path the
// overlays set.
func bootstrapOverlayEvents(cond metav1.Condition, outcome *BootstrapOverlayOutcome) []deployer.ReportEvent {
	// Overlays that are not mounted have no other effect to report.
	if cond.Reason == conditions.KgatewayReasonBootstrapNotMounted {
		return []deployer.ReportEvent{bootstrapOverlayEvent(cond.Reason, cond.Message)}
	}

	// Report each managed path the overlays set.
	var out []deployer.ReportEvent
	for _, p := range outcome.ManagedPaths {
		if outcome.ManagedPathsMode == apisettings.BootstrapOverlayManagedPathsPrune {
			out = append(out, bootstrapOverlayEvent(conditions.KgatewayReasonBootstrapPathsIgnored,
				fmt.Sprintf("Dropped %s from the bootstrap overlay because kgateway manages it.", p)))
			continue
		}
		out = append(out, bootstrapOverlayEvent(conditions.KgatewayReasonBootstrapManagedPathsOverridden,
			fmt.Sprintf("Applied a bootstrap overlay that sets %s, which kgateway manages.", p)))
	}
	return out
}

func bootstrapOverlayEvent(reason, note string) deployer.ReportEvent {
	return deployer.ReportEvent{
		Type:   corev1.EventTypeWarning,
		Reason: reason,
		Action: applyBootstrapOverlaysAction,
		Note:   note,
	}
}

// joinPorts returns ports as a comma-separated list.
func joinPorts(ports []int32) string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, strconv.Itoa(int(p)))
	}
	return strings.Join(out, ", ")
}

func joinManagedPaths(paths []bootstrapoverlay.ManagedPath) string {
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

// findDeployment returns the first rendered Deployment, the proxy's.
func findDeployment(objs []client.Object) *appsv1.Deployment {
	for _, obj := range objs {
		if dep, ok := obj.(*appsv1.Deployment); ok {
			return dep
		}
	}
	return nil
}

// setProxyEnv sets the environment variable name to value on the proxy container of
// each rendered Deployment, replacing any value it has.
func setProxyEnv(objs []client.Object, name, value string) {
	for _, obj := range objs {
		dep, ok := obj.(*appsv1.Deployment)
		if !ok {
			continue
		}
		for i := range dep.Spec.Template.Spec.Containers {
			c := &dep.Spec.Template.Spec.Containers[i]
			if c.Name != wellknown.KgatewayContainerName {
				continue
			}
			c.Env = slices.DeleteFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == name })
			c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
		}
	}
}

// podTemplateAnnotation returns the annotation key on the first rendered Deployment's
// pod template, or "" without one.
func podTemplateAnnotation(objs []client.Object, key string) string {
	dep := findDeployment(objs)
	if dep == nil {
		return ""
	}
	return dep.Spec.Template.Annotations[key]
}

// setPodTemplateAnnotation sets an annotation on each rendered Deployment's pod template.
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

package deployer

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	// bootstrapConfigKey is the key of the Envoy bootstrap in the generated ConfigMap.
	// For a Gateway with bootstrap patches it holds the bootstrap new pods get, and
	// each pod reads its own revision's key instead.
	bootstrapConfigKey = "envoy.yaml"
	// bootstrapMountPath is where the proxy container mounts the generated ConfigMap.
	bootstrapMountPath = "/etc/envoy"
	// bootstrapChecksumAnnotation is the pod-template annotation that holds the SHA-256
	// of the bootstrap, so that a bootstrap change rolls the pods. The Helm template
	// sets it from the rendered bootstrap.
	bootstrapChecksumAnnotation = bootstrappatch.ChecksumAnnotation
	// inputConfigEnv is the proxy container's environment variable that names the
	// bootstrap file envoyinit reads. Without it, envoyinit reads envoy.yaml.
	inputConfigEnv = "INPUT_CONF"
	// maxConfigMapDataSize is the most data, all keys together, the API server accepts
	// in a ConfigMap.
	maxConfigMapDataSize = 1024 * 1024

	// applyBootstrapPatchesAction is the action of each bootstrap patch event.
	applyBootstrapPatchesAction = "ApplyBootstrapPatches"
	// writeBootstrapRevisionAction is the action of the event for a generated bootstrap
	// ConfigMap that would exceed the API server's size limit.
	writeBootstrapRevisionAction = "WriteBootstrapRevision"
	// generatedEntryReplacedEventReason is the reason of the event for an entry of the
	// generated bootstrap, other than a managed path, that bootstrap patches replace.
	generatedEntryReplacedEventReason = "GeneratedEntryReplaced"
)

// BootstrapPatchProvider is an optional interface for a HelmValuesGenerator set with
// GatewayParameters.WithHelmValuesGeneratorOverride. It chooses the bootstrap patches
// for a Gateway. The controller then applies them as it does without an override:
// it merges them into the rendered bootstrap, validates the result, removes the ports
// the bootstrap listens on from the Service, writes the xDS validation data, and
// reports the kgateway.dev/BootstrapPatchApplied condition, events and metric.
//
// The merge runs before the override's PostProcessObjects, when the override
// implements deployer.ObjectPostProcessor, so the post-processor sees the patched
// bootstrap. An override that implements deployer.ObjectPostProcessor without this
// interface replaces the bootstrap patches and the object overlays: no bootstrap
// patches are applied, and no condition is reported. The controller still writes the
// Gateway's bootstrap as its own revision after the post-processor runs.
//
// When a Gateway has bootstrap patches, the override's GetValues should build the
// Service ports with deployer.GetPortsValuesForBootstrapPatches rather than
// deployer.GetPortsValues. Otherwise a listener on a default proxy port, such as
// 8082, is dropped from the Service even when a patch moves the proxy listener off
// that port.
type BootstrapPatchProvider interface {
	// BootstrapOverlays returns the overlays to merge into gw's bootstrap, in order; a
	// later overlay wins a conflict. fromGatewayParameters are the overlays of the
	// GatewayParameters that gw's GatewayClass and gw reference, GatewayClass first.
	// An overlay from another kind of resource sets Source.Kind, so status
	// messages and events name that resource.
	BootstrapOverlays(ctx context.Context, gw *gwv1.Gateway, fromGatewayParameters []bootstrappatch.Overlay) ([]bootstrappatch.Overlay, error)
}

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
	// Proxy is the proxy the deployer rendered and did not deploy, so the reconciler can
	// report a proxy image the failure holds back.
	Proxy RenderedProxy
	Err   error
}

// Events returns the Warning events that report the failure.
func (e *BootstrapPatchError) Events() []deployer.ReportEvent {
	// Report each denied path on its own, so each one can be found.
	if e.Reason == conditions.KgatewayReasonBootstrapPathsDenied {
		out := make([]deployer.ReportEvent, 0, len(e.ManagedPaths))
		for _, p := range e.ManagedPaths {
			out = append(out, bootstrapPatchEvent(e.Reason,
				fmt.Sprintf("Applied no bootstrap patch: a patch sets %s, which the control plane manages, and the controller is configured to reject such patches.", p)))
		}
		return out
	}
	return []deployer.ReportEvent{bootstrapPatchEvent(e.Reason, "Applied no bootstrap patch: "+e.Error())}
}

// pathsKnown reports whether ManagedPaths lists the paths the patches set.
func (e *BootstrapPatchError) pathsKnown() bool {
	return e.Reason != conditions.KgatewayReasonBootstrapInvalidPatch &&
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
	// ReplacedEntries lists each other entry of the generated bootstrap that the
	// patches replace by name. Each replacement applied.
	ReplacedEntries []bootstrappatch.ReplacedEntry
	// RemovedServicePorts lists the Gateway listener ports left off the Service because
	// a static listener in the patched bootstrap uses them.
	RemovedServicePorts []int32
	// RolloutServicePorts lists the Gateway listener ports left off the Service because
	// a static listener in the bootstrap of a running pod uses them, and the patched
	// bootstrap does not. They return to the Service once no running pod uses them.
	RolloutServicePorts []int32

	// checksum is the hex SHA-256 of the patched bootstrap, which names its revision.
	checksum string
	// bootstrap is the patched bootstrap, as written to the ConfigMap.
	bootstrap string
	// validationData is the content of the ConfigMap's validation key.
	validationData string
	// validationAnnotation is the value of the pod template's validation annotation.
	validationAnnotation string
}

// BootstrapPatchConfig is what ApplyBootstrapPatches needs besides the objects and
// the patches.
type BootstrapPatchConfig struct {
	// ListenerPorts are the Gateway's listener ports, which the Service exposes.
	ListenerPorts []int32
	// PodPorts are the ports the bootstraps of the Gateway's running proxy pods listen
	// on. Pods receive the Gateway's listeners until they are replaced, so these ports
	// are left off the Service too.
	PodPorts []uint32
	// ManagedPaths decides what happens to patches that set paths the control plane
	// manages.
	ManagedPaths apisettings.BootstrapPatchManagedPathsMode
	// Validator validates the patched bootstrap with Envoy. When nil, the patched
	// bootstrap is not validated.
	Validator validator.BootstrapValidator
}

// ApplyBootstrapPatches merges overlays, in order, into the bootstrap in the rendered
// bootstrap ConfigMap, validates the result with Envoy, and sets the Deployment's
// checksum annotation to match so the pods roll. It removes from the Service each
// listener port that the patched bootstrap, or the bootstrap of a running pod, also
// listens on. Call it before applying object overlays, so that they can still change
// any object, and then WriteBootstrapRevision once they are applied. It returns
// ctx.Err() when ctx ends, and otherwise a *BootstrapPatchError when the patches are
// not applied, including when every Gateway listener port collides with the patched
// bootstrap.
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
			Err:           fmt.Errorf("the controller failed to apply the bootstrap patch; this is a kgateway defect, not a problem with the patch: %w", err),
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
		return nil, patchErr(conditions.KgatewayReasonBootstrapInvalidPatch, nil, invalid)
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
		return nil, patchErr(conditions.KgatewayReasonBootstrapInvalidPatch, nil, fmt.Errorf(
			"the static runtime layers of the patched bootstrap take %d bytes as JSON, more than the limit of %d; set fewer runtime values in static layers",
			len(subset), bootstrappatch.MaxValidationSubsetSize))
	}

	// Validate the merged bootstrap as the proxy runs it. When ctx has ended, the
	// controller is stopping, and the result says nothing about the patches.
	if cfg.Validator != nil {
		err := cfg.Validator.ValidateBootstrap(ctx, res.Bootstrap)
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
	checksum := hex.EncodeToString(sum[:])
	setPodTemplateAnnotation(rendered, bootstrapChecksumAnnotation, checksum)

	// Encode what the translator validates against, which WriteBootstrapRevision writes:
	// the subset; the ports the bootstrap listens on, where Gateway listeners are
	// rejected; and the static cluster names, which the proxy syncer checks against
	// translated clusters. The pod annotation leaves out the cluster names.
	bootstrapPorts := bootstrappatch.ListenerPorts(res.Bootstrap)
	data, err := bootstrappatch.EncodeValidationData(subset, bootstrapPorts, bootstrappatch.StaticClusterNames(res.Bootstrap))
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to encode the bootstrap validation data: %w", err))
	}
	annotation, err := bootstrappatch.EncodeValidationData(subset, bootstrapPorts, nil)
	if err != nil {
		return nil, controllerErr(fmt.Errorf("failed to encode the bootstrap validation annotation: %w", err))
	}

	// Leave Gateway listener ports the patched bootstrap listens on off the Service.
	// The translator rejects those listeners.
	colliding := listenerPortsIn(cfg.ListenerPorts, bootstrapPorts)
	removed, err := removeServicePorts(rendered, colliding)

	// A Service with no ports cannot be applied, so when every listener port collides
	// none of the objects are, and the proxy keeps running its current bootstrap.
	if errors.Is(err, ErrNoValidPorts) {
		return nil, patchErr(conditions.KgatewayReasonBootstrapInvalidPatch, nil, fmt.Errorf(
			"the patched bootstrap listens on every Gateway listener port (%s), which would leave the Gateway's Service with no ports; move the bootstrap's static listener or add a Gateway listener on another port",
			joinPorts(colliding)))
	}
	if err != nil {
		return nil, controllerErr(err)
	}

	// Also leave off the ports that only a running pod's bootstrap listens on, until no
	// running pod does, unless that would leave the Service with no ports: the rollout
	// that frees them cannot happen while none of the objects are applied.
	var rollout []int32
	if held := slices.DeleteFunc(listenerPortsIn(cfg.ListenerPorts, cfg.PodPorts), func(p int32) bool {
		return slices.Contains(colliding, p)
	}); servicesKeepPorts(rendered, held) {
		rollout, err = removeServicePorts(rendered, held)
		if err != nil {
			return nil, controllerErr(err)
		}
	}

	return &BootstrapPatchOutcome{
		ConfigMapName:        cm.Name,
		ManagedPathsMode:     mode,
		ManagedPaths:         res.ManagedPaths,
		ReplacedEntries:      res.ReplacedEntries,
		RemovedServicePorts:  removed,
		RolloutServicePorts:  rollout,
		checksum:             checksum,
		bootstrap:            string(merged),
		validationData:       data,
		validationAnnotation: annotation,
	}, nil
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
}

// BootstrapConfigMapTooLargeError reports a generated bootstrap ConfigMap that would
// exceed the API server's size limit with the revisions it has to hold. None of the
// Gateway's objects are deployed. For a Gateway with bootstrap patches it is wrapped in
// a *BootstrapPatchError; for a Gateway without them it is returned as is, and the
// reconciler reports it as a Warning event.
type BootstrapConfigMapTooLargeError struct {
	// ConfigMapName is the name of the Gateway's generated bootstrap ConfigMap.
	ConfigMapName string
	// Proxy is the proxy the deployer rendered and did not deploy, so the reconciler can
	// report a proxy image the failure holds back.
	Proxy RenderedProxy
	Err   error
}

// Events returns the Warning event that reports the failure for a Gateway without
// bootstrap patches.
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

// WriteBootstrapRevision writes the patched bootstrap that ApplyBootstrapPatches
// produced as its own revision: under the envoy-<checksum>.yaml key of the generated
// ConfigMap, which INPUT_CONF on the proxy container points at, so a pod always reads
// the bootstrap its annotations describe, even when its container restarts during a
// rollout. It also writes the validation data to the ConfigMap's validation key and the
// pod template. It keeps each earlier revision in revs.InUse that the live ConfigMap
// holds, and leaves out the rest, so applying the ConfigMap removes them. Call it
// after the object overlays, and only when the proxy mounts the generated ConfigMap
// (see BootstrapMountProblem). It returns a *BootstrapPatchError when the ConfigMap
// would exceed the API server's size limit.
func WriteBootstrapRevision(objs []client.Object, outcome *BootstrapPatchOutcome, revs BootstrapRevisions) error {
	// Write the validation data.
	cm := findBootstrapConfigMap(objs)
	if cm == nil {
		return &BootstrapPatchError{
			Reason: conditions.KgatewayReasonBootstrapControllerError,
			Err:    errors.New("the controller failed to apply the bootstrap patch; this is a kgateway defect, not a problem with the patch: no bootstrap ConfigMap was rendered"),
		}
	}
	cm.Data[bootstrappatch.ValidationKey] = outcome.validationData
	setPodTemplateAnnotation(objs, bootstrappatch.ValidationAnnotation, outcome.validationAnnotation)

	// Write this revision and the earlier ones in use.
	return outcome.wrapTooLarge(writeBootstrapRevisions(objs, cm, outcome.checksum, outcome.bootstrap, revs, true))
}

// WriteGeneratedBootstrapRevision is WriteBootstrapRevision for a Gateway without
// bootstrap patches: it writes the generated bootstrap as its own revision, named by
// the pod template's checksum annotation, and points INPUT_CONF at it, so adding or
// removing a patch changes only the content of the next revision. It writes no
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

// KeepBootstrapRevisions is for a proxy that does not mount the generated ConfigMap
// (see BootstrapMountProblem): it writes no revision for the current bootstrap and
// leaves INPUT_CONF unset, so envoyinit reads envoy.yaml from what the proxy mounts.
// It keeps each earlier revision in revs.InUse that the live ConfigMap holds, so pods
// that still mount the generated ConfigMap keep reading their own. outcome is nil for
// a Gateway without bootstrap patches. It returns a *BootstrapPatchError, or without
// patches a *BootstrapConfigMapTooLargeError, when the ConfigMap would exceed the API
// server's size limit.
func KeepBootstrapRevisions(objs []client.Object, outcome *BootstrapPatchOutcome, revs BootstrapRevisions) error {
	cm := findBootstrapConfigMap(objs)
	if cm == nil {
		return nil
	}
	return outcome.wrapTooLarge(writeBootstrapRevisions(objs, cm, "", cm.Data[bootstrapConfigKey], revs, outcome != nil))
}

// writeBootstrapRevisions writes bootstrap under the revision key for checksum and
// points INPUT_CONF on the proxy container at it; with an empty checksum it writes
// neither. It then copies each earlier revision in revs.InUse that the live ConfigMap
// holds, and checks the API server's size limit. A revision in use is never dropped
// to make room: a pod reading it would wait for it when its container restarts.
// patched says whether the Gateway has bootstrap patches, for the error message.
func writeBootstrapRevisions(objs []client.Object, cm *corev1.ConfigMap, checksum, bootstrap string, revs BootstrapRevisions, patched bool) error {
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
	case patched:
		msg += "; make the bootstrap patch smaller"
	}
	return &BootstrapConfigMapTooLargeError{ConfigMapName: cm.Name, Err: errors.New(msg)}
}

// wrapTooLarge returns err as a *BootstrapPatchError when it is a
// *BootstrapConfigMapTooLargeError and the Gateway has bootstrap patches (o is not
// nil), so it is reported on the kgateway.dev/BootstrapPatchApplied condition.
// Otherwise it returns err unchanged.
func (o *BootstrapPatchOutcome) wrapTooLarge(err error) error {
	tooLarge, ok := errors.AsType[*BootstrapConfigMapTooLargeError](err)
	if !ok || o == nil {
		return err
	}
	return &BootstrapPatchError{
		Reason:        conditions.KgatewayReasonBootstrapConfigMapTooLarge,
		ConfigMapName: tooLarge.ConfigMapName,
		ManagedPaths:  o.ManagedPaths,
		Err:           tooLarge,
	}
}

// BootstrapMountProblem returns why the proxy container of the Deployment in objs does
// not read the generated ConfigMap cmName at /etc/envoy, or "" when it does. An object
// overlay can point the bootstrap volume at another ConfigMap, or mount another volume
// there, in which case the proxy does not run the patched bootstrap.
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
	return ""
}

// bootstrapRevisionKey returns the generated ConfigMap's key for the bootstrap with
// checksum sum.
func bootstrapRevisionKey(sum string) string {
	return "envoy-" + sum + ".yaml"
}

// configMapDataSize returns the size the API server limits: the values of all keys
// together.
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

// bootstrapOverlays returns the bootstrap patches of the resolved GatewayParameters,
// GatewayClass first, so that the Gateway's overlay wins a conflict.
func (r *resolvedKgatewayParameters) bootstrapOverlays() []bootstrappatch.Overlay {
	var out []bootstrappatch.Overlay
	for _, gwp := range []*kgateway.GatewayParameters{r.gatewayClassGWP, r.gatewayGWP} {
		if gwp == nil {
			continue
		}
		overlay := gwp.Spec.GetKube().GetEnvoyContainer().GetBootstrap().GetPatch()
		if overlay == nil {
			continue
		}

		// A GatewayParameters referenced by both the GatewayClass and the Gateway applies once.
		source := bootstrappatch.Source{Namespace: gwp.Namespace, Name: gwp.Name}
		if slices.ContainsFunc(out, func(o bootstrappatch.Overlay) bool { return o.Source == source }) {
			continue
		}
		out = append(out, bootstrappatch.Overlay{Source: source, Raw: overlay.Value.Raw})
	}
	return out
}

// bootstrapPatchAppliedCondition reports patches that were applied. mountProblem is
// BootstrapMountProblem, and imageNote is proxyImageNote, of the final objects, after
// the object overlays.
func bootstrapPatchAppliedCondition(gw *gwv1.Gateway, outcome *BootstrapPatchOutcome, mountProblem, imageNote string) metav1.Condition {
	cond := metav1.Condition{
		Type:               conditions.KgatewayConditionBootstrapPatchApplied,
		ObservedGeneration: gw.Generation,
	}

	// An object overlay that mounts something else at /etc/envoy keeps the patches off
	// the proxy.
	if mountProblem != "" {
		cond.Status = metav1.ConditionFalse
		cond.Reason = conditions.KgatewayReasonBootstrapNotMounted
		cond.Message = fmt.Sprintf("The bootstrap patch is not in effect: %s. Routes are validated against Envoy's defaults, not the patch.", mountProblem)
		if imageNote != "" {
			cond.Message += " " + imageNote
		}
		return cond
	}

	// Report each managed path the patches set: under PRUNE it was dropped, otherwise
	// it applied.
	cond.Status = metav1.ConditionTrue
	cond.Reason = conditions.KgatewayReasonBootstrapPatchApplied
	msg := []string{"The bootstrap patch is applied."}
	switch {
	case len(outcome.ManagedPaths) == 0:
	case outcome.ManagedPathsMode == apisettings.BootstrapPatchManagedPathsPrune:
		cond.Reason = conditions.KgatewayReasonBootstrapPathsIgnored
		msg = append(msg, fmt.Sprintf(
			"These paths were dropped from it because the control plane manages them: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	default:
		cond.Reason = conditions.KgatewayReasonBootstrapManagedPathsOverridden
		msg = append(msg, fmt.Sprintf(
			"It sets paths the control plane manages, which can disconnect the proxy from the controller or break its probes: %s.",
			joinManagedPaths(outcome.ManagedPaths)))
	}

	// Report each other generated entry the patches replace. The reason does not change.
	if len(outcome.ReplacedEntries) > 0 {
		msg = append(msg, fmt.Sprintf(
			"It replaces these entries of the generated bootstrap, so the proxy runs the patch's version of each instead of kgateway's: %s.",
			joinReplacedEntries(outcome.ReplacedEntries)))
	}

	// Report each listener port left off the Service.
	if len(outcome.RemovedServicePorts) > 0 {
		msg = append(msg, fmt.Sprintf(
			"A static listener in the bootstrap uses Gateway listener port(s) %s, so those ports are left off the Service and the Gateway listeners on them are rejected.",
			joinPorts(outcome.RemovedServicePorts)))
	}
	if len(outcome.RolloutServicePorts) > 0 {
		msg = append(msg, fmt.Sprintf(
			"Proxy pods still running an earlier bootstrap listen on Gateway listener port(s) %s, so those ports are left off the Service, and the Gateway listeners on them are rejected, until the rollout completes.",
			joinPorts(outcome.RolloutServicePorts)))
	}

	// Report a proxy image that differs from the controller's. The reason does not
	// change.
	if imageNote != "" {
		msg = append(msg, imageNote)
	}
	cond.Message = strings.Join(msg, " ")
	return cond
}

// bootstrapPatchEvents returns the Warning events that go with the
// kgateway.dev/BootstrapPatchApplied condition cond: one for each managed path the
// patches set, and one for each other generated entry they replace.
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
				fmt.Sprintf("Dropped %s from the bootstrap patch because the control plane manages it.", p)))
			continue
		}
		out = append(out, bootstrapPatchEvent(conditions.KgatewayReasonBootstrapManagedPathsOverridden,
			fmt.Sprintf("Applied a bootstrap patch that sets %s, which the control plane manages.", p)))
	}

	// Report each other generated entry the patches replace.
	for _, e := range outcome.ReplacedEntries {
		out = append(out, bootstrapPatchEvent(generatedEntryReplacedEventReason,
			fmt.Sprintf("Applied a bootstrap patch that replaces %s, an entry of the generated bootstrap.", e)))
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

// joinPorts returns ports as a comma-separated list.
func joinPorts(ports []int32) string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, strconv.Itoa(int(p)))
	}
	return strings.Join(out, ", ")
}

func joinManagedPaths(paths []bootstrappatch.ManagedPath) string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, p.String())
	}
	return strings.Join(out, ", ")
}

func joinReplacedEntries(entries []bootstrappatch.ReplacedEntry) string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.String())
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

// servicesKeepPorts reports whether every rendered Service keeps a port after the
// ports whose target port is in ports are removed.
func servicesKeepPorts(objs []client.Object, ports []int32) bool {
	for _, obj := range objs {
		svc, ok := obj.(*corev1.Service)
		if !ok {
			continue
		}
		if !slices.ContainsFunc(svc.Spec.Ports, func(p corev1.ServicePort) bool { return !slices.Contains(ports, p.TargetPort.IntVal) }) {
			return false
		}
	}
	return true
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

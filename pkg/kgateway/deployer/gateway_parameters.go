package deployer

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"helm.sh/helm/v3/pkg/chart"
	"istio.io/istio/pkg/kube/kclient"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer/strategicpatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

var (
	logger = logging.New("gateway-deployer")

	// ErrNoValidPorts is returned when no valid ports are found for the Gateway
	ErrNoValidPorts = errors.New("no valid ports")

	// ErrNotFound is returned when a requested resource is not found
	ErrNotFound = errors.New("resource not found")

	// ErrBootstrapConfigMapUnreadable is returned when the controller cannot read a
	// Gateway's generated bootstrap ConfigMap from the API server. The Gateway's
	// objects are left unchanged and the reconcile is retried.
	ErrBootstrapConfigMapUnreadable = errors.New("could not read the generated bootstrap ConfigMap")
)

func NewGatewayParameters(cli apiclient.Client, inputs *deployer.Inputs) *GatewayParameters {
	gp := &GatewayParameters{
		inputs: inputs,
	}

	gp.kgwParameters = newkgatewayParameters(cli, inputs)

	return gp
}

type GatewayParameters struct {
	inputs                      *deployer.Inputs
	helmValuesGeneratorOverride deployer.HelmValuesGenerator
	kgwParameters               *kgatewayParameters
}

type kgatewayParameters struct {
	gwParamClient kclient.Client[*kgateway.GatewayParameters]
	gwClassClient kclient.Client[*gwv1.GatewayClass]
	// replicaSetClient lists the ReplicaSets of proxy Deployments, to keep the
	// bootstrap revisions they can still create pods with.
	replicaSetClient kclient.Client[*appsv1.ReplicaSet]
	// configMapClient is the informer copy of the generated bootstrap ConfigMaps, read
	// first for the bootstrap revisions in use.
	configMapClient kclient.Client[*corev1.ConfigMap]
	// getConfigMap reads a ConfigMap from the API server, for when the informer copy
	// lacks a bootstrap revision in use.
	getConfigMap func(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error)
	inputs       *deployer.Inputs
}

func (gp *GatewayParameters) WithHelmValuesGeneratorOverride(generator deployer.HelmValuesGenerator) *GatewayParameters {
	gp.helmValuesGeneratorOverride = generator
	return gp
}

// GetGatewayParametersClient returns the GatewayParameters client.
// This allows the reconciler to reuse the same client for watching changes.
func (gp *GatewayParameters) GetGatewayParametersClient() kclient.Client[*kgateway.GatewayParameters] {
	return gp.kgwParameters.gwParamClient
}

// GetReplicaSetClient returns the client for the ReplicaSets of proxy Deployments.
// The reconciler watches it to remove a bootstrap revision once no ReplicaSet can
// create a pod with it.
func (gp *GatewayParameters) GetReplicaSetClient() kclient.Client[*appsv1.ReplicaSet] {
	return gp.kgwParameters.replicaSetClient
}

func LoadEnvoyChart() (*chart.Chart, error) {
	return loadChart(helm.EnvoyHelmChart)
}

func (gp *GatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	generator, err := gp.getHelmValuesGenerator(obj)
	if err != nil {
		return nil, err
	}

	return generator.GetValues(ctx, obj)
}

func (gp *GatewayParameters) GetCacheSyncHandlers() []cache.InformerSynced {
	// The bootstrap revisions read ReplicaSets and ConfigMaps with or without an override.
	// The handlers go into a new slice, so the override's slice is not written to.
	if gp.helmValuesGeneratorOverride != nil {
		return slices.Concat(gp.helmValuesGeneratorOverride.GetCacheSyncHandlers(),
			[]cache.InformerSynced{gp.kgwParameters.replicaSetClient.HasSynced, gp.kgwParameters.configMapClient.HasSynced})
	}

	return gp.kgwParameters.GetCacheSyncHandlers()
}

// PostProcessObjects implements deployer.ObjectPostProcessor. It is
// PostProcessObjectsWithReport without the report.
func (gp *GatewayParameters) PostProcessObjects(ctx context.Context, obj client.Object, rendered []client.Object) ([]client.Object, error) {
	objs, _, err := gp.PostProcessObjectsWithReport(ctx, obj, rendered)
	return objs, err
}

// PostProcessObjectsWithReport implements deployer.ReportingObjectPostProcessor.
// It merges the GatewayParameters bootstrap overlays into the rendered bootstrap, then
// applies the GatewayParameters object overlays to the rendered objects. Each is
// applied in order, GatewayClass first and Gateway on top. The report carries the
// kgateway.dev/BootstrapOverlayApplied condition when there are bootstrap overlays.
//
// An override HelmValuesGenerator that implements deployer.ObjectPostProcessor
// replaces the object overlays. When it also implements BootstrapOverlayProvider, it
// chooses the bootstrap overlays and its post-processor runs after the merge;
// otherwise its post-processor replaces the bootstrap overlays and the object overlays, and
// there is no report. Either way, every Gateway's bootstrap is then written as its
// own revision (see WriteGeneratedBootstrapRevision).
func (gp *GatewayParameters) PostProcessObjectsWithReport(ctx context.Context, obj client.Object, rendered []client.Object) ([]client.Object, *deployer.PostProcessReport, error) {
	// Find which steps the override takes over.
	overridePostProcessor, hasOverridePostProcessor := gp.helmValuesGeneratorOverride.(deployer.ObjectPostProcessor)
	overlayProvider, hasOverlayProvider := gp.helmValuesGeneratorOverride.(BootstrapOverlayProvider)

	// An override that post-processes without choosing bootstrap overlays handles the
	// objects itself, as it does for any object that is not a Gateway. Its Gateways
	// still get bootstrap revisions, written after it so the mount check sees the
	// volumes it sets. It applies no bootstrap overlays, so the report removes a
	// kgateway.dev/BootstrapOverlayApplied condition an earlier controller left.
	gw, isGateway := obj.(*gwv1.Gateway)
	if hasOverridePostProcessor && (!hasOverlayProvider || !isGateway) {
		objs, err := overridePostProcessor.PostProcessObjects(ctx, obj, rendered)
		if err != nil || !isGateway {
			return objs, nil, err
		}
		gwNN := types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}
		if _, err := gp.writeBootstrapRevisions(ctx, gwNN, objs, nil, gp.runningBootstraps(gwNN)); err != nil {
			return nil, nil, withRenderedProxy(err, objs)
		}
		return objs, &deployer.PostProcessReport{OwnedConditionTypes: []string{conditions.KgatewayConditionBootstrapOverlayApplied}}, nil
	}
	if !isGateway {
		return rendered, nil, nil
	}

	// Resolve the GatewayParameters of the GatewayClass and the Gateway.
	resolved := &resolvedKgatewayParameters{}
	if gp.kgwParameters != nil {
		resolved = gp.kgwParameters.resolveParametersForOverlays(gw)
	}
	report := &deployer.PostProcessReport{
		OwnedConditionTypes: []string{conditions.KgatewayConditionBootstrapOverlayApplied},
	}

	// Collect the bootstrap overlays, letting an override that provides them choose.
	gwNN := types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}
	overlays := resolved.bootstrapOverlays()
	if hasOverlayProvider {
		var err error
		overlays, err = overlayProvider.BootstrapOverlays(ctx, gw, overlays)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get the bootstrap overlay for Gateway %s: %w", gwNN, err)
		}
	}

	// Post-process the objects with the override's post-processor when it has one,
	// otherwise the object overlays.
	postProcess := func(objs []client.Object) ([]client.Object, error) {
		if hasOverridePostProcessor {
			return overridePostProcessor.PostProcessObjects(ctx, gw, objs)
		}
		return resolved.applyObjectOverlays(objs)
	}

	// Merge the bootstrap overlays first, so the object post-processing can still change any object.
	var outcome *BootstrapOverlayOutcome
	running := gp.runningBootstraps(gwNN)
	if len(overlays) > 0 {
		var err error
		outcome, err = ApplyBootstrapOverlays(ctx, rendered, overlays, BootstrapOverlayConfig{
			ManagedPaths: gp.inputs.BootstrapOverlayManagedPaths,
			Validator:    gp.inputs.BootstrapValidator,
		})

		// When the managed paths are unknown, the series stay as they were.
		if overlayErr, ok := errors.AsType[*BootstrapOverlayError](err); ok && overlayErr.pathsKnown() {
			recordBootstrapOverlayManagedPaths(gwNN, overlayErr.ManagedPaths)
		}

		// Name the proxy the failure holds back, after the post-processing when it
		// applies to the objects the overlays were not merged into, and say when that
		// proxy does not read the generated bootstrap, so the overlays could not reach it.
		// A post-processing failure is recorded on the overlay error, which is the one
		// returned.
		if err != nil {
			overlayErr, isOverlayErr := errors.AsType[*BootstrapOverlayError](err)
			final, ppErr := postProcess(rendered)
			if ppErr != nil {
				if isOverlayErr {
					overlayErr.ObjectsErr = ppErr
				}
				return nil, nil, withRenderedProxy(err, rendered)
			}
			if isOverlayErr && overlayErr.ConfigMapName != "" {
				overlayErr.MountProblem = BootstrapMountProblem(final, overlayErr.ConfigMapName)
			}
			return nil, nil, withRenderedProxy(err, final)
		}
		recordBootstrapOverlayManagedPaths(gwNN, outcome.ManagedPaths)
	} else {
		ForgetBootstrapOverlayMetrics(gwNN)
	}

	// Post-process the objects.
	rendered, err := postProcess(rendered)
	if err != nil {
		return nil, nil, err
	}

	// Write the bootstrap's revision, and the overlays' validation data.
	mountProblem, err := gp.writeBootstrapRevisions(ctx, gwNN, rendered, outcome, running)
	if err != nil {
		return nil, nil, withRenderedProxy(err, rendered)
	}
	if outcome == nil {
		return rendered, report, nil
	}

	// Leave the ports the merged bootstrap listens on off the Service. A proxy that
	// does not run it listens on the default ports, which the Service never has.
	if mountProblem == "" {
		listenerPorts := deployer.GetGatewayIR(gw, gp.inputs.CommonCollections).Ports.List()
		if err := ReserveServicePorts(rendered, outcome, listenerPorts); err != nil {
			return nil, nil, withRenderedProxy(err, rendered)
		}
	}

	// Report the bootstrap overlays against the final objects.
	cond := bootstrapOverlayAppliedCondition(gw, outcome, mountProblem)
	report.Conditions = append(report.Conditions, cond)
	report.Events = append(report.Events, bootstrapOverlayEvents(cond, outcome)...)
	return rendered, report, nil
}

// withRenderedProxy records on a *BootstrapOverlayError or *BootstrapConfigMapTooLargeError
// the proxy in objs, which the failure keeps from being deployed, and returns err.
func withRenderedProxy(err error, objs []client.Object) error {
	if overlayErr, ok := errors.AsType[*BootstrapOverlayError](err); ok {
		overlayErr.Proxy = renderedProxy(objs)
	}
	if tooLarge, ok := errors.AsType[*BootstrapConfigMapTooLargeError](err); ok {
		tooLarge.Proxy = renderedProxy(objs)
	}
	return err
}

// writeBootstrapRevisions writes the Gateway's current bootstrap revision into the
// final objects, keeps the earlier revisions its pods and ReplicaSets still use, and
// returns BootstrapMountProblem of the objects. outcome is nil for a Gateway without
// bootstrap overlays.
//
// When the proxy reads the generated ConfigMap, the current revision is written, with
// the overlays' validation data when there are overlays. Otherwise the proxy does not
// read the generated bootstrap, so no current revision or validation data is written,
// and routes are validated against Envoy's defaults and the default ports; the earlier
// revisions are still kept for the pods that mount the generated ConfigMap.
func (gp *GatewayParameters) writeBootstrapRevisions(
	ctx context.Context,
	gw types.NamespacedName,
	objs []client.Object,
	outcome *BootstrapOverlayOutcome,
	running krtcollections.GatewayRunningBootstraps,
) (string, error) {
	// Find the generated ConfigMap. Without overlays, a Gateway that renders none has no revisions.
	var cmName string
	switch cm := findBootstrapConfigMap(objs); {
	case outcome != nil:
		cmName = outcome.ConfigMapName
	case cm != nil:
		cmName = cm.Name
	default:
		return "", nil
	}

	// Find the revision this reconcile writes: none when the proxy does not read the
	// generated ConfigMap.
	mountProblem := BootstrapMountProblem(objs, cmName)
	var current string
	switch {
	case mountProblem != "":
	case outcome != nil:
		current = outcome.checksum
	default:
		current = podTemplateAnnotation(objs, bootstrapChecksumAnnotation)
	}

	// Read the revisions in use.
	revs, err := gp.bootstrapRevisions(ctx, gw, objs, cmName, running.Revisions, current)
	if err != nil {
		return mountProblem, err
	}

	// Write the current revision when the proxy reads it, and keep the earlier ones.
	switch {
	case mountProblem != "":
		err = KeepBootstrapRevisions(objs, outcome, revs)
	case outcome != nil:
		err = WriteBootstrapRevision(objs, outcome, revs)
	default:
		err = WriteGeneratedBootstrapRevision(objs, revs)
	}
	return mountProblem, err
}

// runningBootstraps returns what the Gateway's running proxy pods say about the
// bootstraps they run, or nothing when the controller does not collect it.
func (gp *GatewayParameters) runningBootstraps(gw types.NamespacedName) krtcollections.GatewayRunningBootstraps {
	cols := gp.inputs.CommonCollections
	if cols == nil || cols.GatewayIndex == nil || cols.GatewayIndex.RunningBootstraps == nil {
		return krtcollections.GatewayRunningBootstraps{Gateway: gw}
	}
	if rb := cols.GatewayIndex.RunningBootstraps.GetKey(gw.String()); rb != nil {
		return *rb
	}
	return krtcollections.GatewayRunningBootstraps{Gateway: gw}
}

// bootstrapRevisions returns the Gateway's live generated ConfigMap cmName and the
// bootstrap revisions in use: podRevisions, from the Gateway's running pods, and those
// of the ReplicaSets of the rendered proxy Deployment with replicas above zero. current
// is the revision this reconcile writes, or "" when it writes none.
//
// It also returns, as Previous, the revision the live ConfigMap is current on. The
// proxy Deployment's template names that revision until this reconcile's objects are
// applied, and a ReplicaSet or pod just created from the template may not be in the
// controller's informers yet, so WriteBootstrapRevision keeps it once more.
//
// WriteBootstrapRevision keeps only the earlier revisions the returned ConfigMap holds,
// so a stale copy that lacks one would drop the key a pod reads when its container
// restarts. A revision's key names the SHA-256 of its content, so a stale copy that
// holds the key holds the right content. A copy that lacks the current revision may be
// from before the last write, and would then name an older revision as Previous. The
// ConfigMap is therefore read from the informer, and from the API server only when the
// informer copy lacks the current revision or an earlier revision in use: once for
// each change of a Gateway's bootstrap, and again until the informer has the write.
// A proxy with a mount problem reads it on every reconcile: its pods' revision is
// never written, so the informer copy always lacks it. Any error from the API server
// other than NotFound fails with ErrBootstrapConfigMapUnreadable rather than reading
// as no ConfigMap, which would drop every earlier revision.
func (gp *GatewayParameters) bootstrapRevisions(ctx context.Context, gw types.NamespacedName, rendered []client.Object, cmName string, podRevisions []string, current string) (BootstrapRevisions, error) {
	// Collect the revisions in use: the pods', and those of each ReplicaSet of the proxy
	// Deployment that can still create a pod.
	out := BootstrapRevisions{InUse: slices.Clone(podRevisions)}
	if gp.kgwParameters == nil {
		return out, fmt.Errorf("%w %s/%s: the deployer has no API client", ErrBootstrapConfigMapUnreadable, gw.Namespace, cmName)
	}
	if dep := findDeployment(rendered); dep != nil {
		selector := labels.SelectorFromSet(labels.Set{wellknown.GatewayNameLabel: kubeutils.SafeGatewayLabelValue(gw.Name)})
		for _, rs := range gp.kgwParameters.replicaSetClient.List(gw.Namespace, selector) {
			owner := metav1.GetControllerOf(rs)
			if owner == nil || owner.Kind != wellknown.DeploymentGVK.Kind || owner.Name != dep.Name {
				continue
			}
			if ptr.Deref(rs.Spec.Replicas, 1) > 0 {
				out.InUse = append(out.InUse, rs.Spec.Template.Annotations[bootstrapChecksumAnnotation])
			}
		}
	}

	// Use the informer copy when it holds the current revision and every earlier
	// revision in use.
	cached := gp.kgwParameters.configMapClient.Get(cmName, gw.Namespace)
	missing := missingRevisions(cached, append(slices.Clone(out.InUse), current))
	if len(missing) == 0 {
		out.Live = cached
		out.Previous = currentBootstrapRevision(cached)
		return out, nil
	}

	// Otherwise read the ConfigMap from the API server.
	logger.Debug("reading the generated bootstrap ConfigMap from the API server, because the informer copy lacks the current revision or revisions in use",
		"configmap", gw.Namespace+"/"+cmName, "revisions", missing)
	live, err := gp.kgwParameters.getConfigMap(ctx, gw.Namespace, cmName)
	switch {
	case apierrors.IsNotFound(err):
	case err != nil:
		return out, fmt.Errorf("%w %s/%s: %w", ErrBootstrapConfigMapUnreadable, gw.Namespace, cmName, err)
	default:
		out.Live = live
		out.Previous = currentBootstrapRevision(live)
	}
	return out, nil
}

// missingRevisions returns each revision in sums that cm has no key for. cm may be nil.
func missingRevisions(cm *corev1.ConfigMap, sums []string) []string {
	var missing []string
	for _, sum := range slices.Compact(slices.Sorted(slices.Values(sums))) {
		if sum == "" {
			continue
		}
		if cm == nil {
			missing = append(missing, sum)
			continue
		}
		if _, ok := cm.Data[bootstrapRevisionKey(sum)]; !ok {
			missing = append(missing, sum)
		}
	}
	return missing
}

// applyObjectOverlays applies the resolved GatewayParameters object overlays to objs,
// GatewayClass first, then Gateway.
func (r *resolvedKgatewayParameters) applyObjectOverlays(objs []client.Object) ([]client.Object, error) {
	for _, gwp := range []*kgateway.GatewayParameters{r.gatewayClassGWP, r.gatewayGWP} {
		if gwp == nil {
			continue
		}
		var err error
		objs, err = strategicpatch.NewOverlayApplierFromGatewayParameters(gwp).ApplyOverlays(objs)
		if err != nil {
			return nil, err
		}
	}
	return objs, nil
}

func GatewayReleaseNameAndNamespace(obj client.Object) (string, string) {
	// A helm release is never installed, only a template is generated, so the name doesn't matter
	// Use a hard-coded name to avoid going over the 53 character name limit
	return "release-name-placeholder", obj.GetNamespace()
}

func (gp *GatewayParameters) getHelmValuesGenerator(obj client.Object) (deployer.HelmValuesGenerator, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	if gp.helmValuesGeneratorOverride != nil {
		logger.Debug("using override HelmValuesGenerator for Gateway",
			"gateway_name", gw.GetName(),
			"gateway_namespace", gw.GetNamespace(),
		)
		return gp.helmValuesGeneratorOverride, nil
	}

	if gp.kgwParameters == nil {
		return nil, errors.New("no parameter clients available")
	}
	logger.Debug("using default HelmValuesGenerator for Gateway",
		"gateway_name", gw.GetName(),
		"gateway_namespace", gw.GetNamespace(),
	)
	return gp.kgwParameters, nil
}

func newkgatewayParameters(cli apiclient.Client, inputs *deployer.Inputs) *kgatewayParameters {
	return &kgatewayParameters{
		gwParamClient: kclient.NewFilteredDelayed[*kgateway.GatewayParameters](cli, wellknown.GatewayParametersGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		gwClassClient: kclient.NewFilteredDelayed[*gwv1.GatewayClass](cli, wellknown.GatewayClassGVR, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		replicaSetClient: kclient.NewFiltered[*appsv1.ReplicaSet](cli, kclient.Filter{
			ObjectFilter:  cli.ObjectFilter(),
			LabelSelector: wellknown.GatewayNameLabel,
		}),
		configMapClient: kclient.NewFiltered[*corev1.ConfigMap](cli, kclient.Filter{ObjectFilter: cli.ObjectFilter()}),
		getConfigMap: func(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
			return cli.Kube().CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		},
		inputs: inputs,
	}
}

func (h *kgatewayParameters) GetValues(ctx context.Context, obj client.Object) (map[string]any, error) {
	gw, ok := obj.(*gwv1.Gateway)
	if !ok {
		return nil, fmt.Errorf("expected a Gateway resource, got %s", obj.GetObjectKind().GroupVersionKind().String())
	}

	gwParam, err := h.getGatewayParametersForGateway(gw)
	if err != nil {
		return nil, err
	}
	// If this is a self-managed Gateway, skip gateway auto provisioning
	if gwParam != nil && gwParam.Spec.SelfManaged != nil {
		return nil, nil
	}
	vals, err := h.getValues(gw, gwParam)
	if err != nil {
		return nil, err
	}

	var jsonVals map[string]any
	err = deployer.JsonConvert(vals, &jsonVals)
	return jsonVals, err
}

func (k *kgatewayParameters) GetCacheSyncHandlers() []cache.InformerSynced {
	return []cache.InformerSynced{k.gwClassClient.HasSynced, k.gwParamClient.HasSynced, k.replicaSetClient.HasSynced, k.configMapClient.HasSynced}
}

// getGatewayParametersForGateway returns the merged GatewayParameters object resulting from the default GwParams object and
// the GwParam object specifically associated with the given Gateway (if one exists).
func (k *kgatewayParameters) getGatewayParametersForGateway(gw *gwv1.Gateway) (*kgateway.GatewayParameters, error) {
	// attempt to get the GatewayParameters name from the Gateway. If we can't find it,
	// we'll check for the default GWP for the GatewayClass.
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		logger.Debug("no GatewayParameters found for Gateway, using default",
			"gateway_name", gw.GetName(),
			"gateway_namespace", gw.GetNamespace(),
		)
		return k.getDefaultGatewayParameters(gw)
	}

	ref := gw.Spec.Infrastructure.ParametersRef

	gwpName := ref.Name
	if group := ref.Group; group != kgateway.GroupName {
		return nil, fmt.Errorf("invalid group %s for GatewayParameters", group)
	}
	if kind := ref.Kind; kind != gwv1.Kind(wellknown.GatewayParametersGVK.Kind) {
		return nil, fmt.Errorf("invalid kind %s for GatewayParameters", kind)
	}

	// the GatewayParameters must live in the same namespace as the Gateway
	gwpNamespace := gw.GetNamespace()
	gwp := k.gwParamClient.Get(gwpName, gwpNamespace)
	if gwp == nil {
		return nil, deployer.GetGatewayParametersForGatewayError(ErrNotFound, gwpNamespace, gwpName, gw.GetNamespace(), gw.GetName(), "Gateway")
	}

	// Propagate the Gateway-level omitDefaultSecurityContext flag so that
	// in-memory defaults are generated without security context from the
	// start. Do not discard envoy securityContext fields set by the
	// GatewayClass params.
	omitDefaultSecurityContext := ptr.Deref(gwp.Spec.Kube.GetOmitDefaultSecurityContext(), false)

	defaultGwp, err := k.getDefaultGatewayParametersWithFlag(gw, omitDefaultSecurityContext)
	if err != nil {
		return nil, err
	}

	mergedGwp := defaultGwp
	deployer.DeepMergeGatewayParameters(mergedGwp, gwp)
	return mergedGwp, nil
}

// gets the default GatewayParameters associated with the GatewayClass of the provided Gateway
func (k *kgatewayParameters) getDefaultGatewayParameters(gw *gwv1.Gateway) (*kgateway.GatewayParameters, error) {
	return k.getDefaultGatewayParametersWithFlag(gw, false)
}

// getDefaultGatewayParametersWithFlag gets the GatewayClass-level effective
// parameters, honoring the given omitDefaultSecurityContext flag.
func (k *kgatewayParameters) getDefaultGatewayParametersWithFlag(gw *gwv1.Gateway, omitDefaultSecurityContext bool) (*kgateway.GatewayParameters, error) {
	gwc, err := getGatewayClassFromGateway(k.gwClassClient, gw)
	if err != nil {
		return nil, err
	}

	paramRef := gwc.Spec.ParametersRef
	if paramRef == nil {
		return deployer.GetInMemoryGatewayParameters(deployer.InMemoryGatewayParametersConfig{
			ControllerName:             string(gwc.Spec.ControllerName),
			ClassName:                  gwc.GetName(),
			ImageInfo:                  k.inputs.ImageInfo,
			OmitDefaultSecurityContext: omitDefaultSecurityContext,
		})
	}

	gwcParams, err := k.resolveGatewayClassParameters(gwc)
	if err != nil {
		return nil, err
	}

	omit := omitDefaultSecurityContext || ptr.Deref(gwcParams.Spec.Kube.GetOmitDefaultSecurityContext(), false)

	return k.mergeWithDefaults(gwc, gwcParams, omit)
}

// resolveGatewayClassParameters fetches the raw GatewayParameters for a
// GatewayClass without merging with defaults.
func (k *kgatewayParameters) resolveGatewayClassParameters(gwc *gwv1.GatewayClass) (*kgateway.GatewayParameters, error) {
	paramRef := gwc.Spec.ParametersRef

	gwpName := paramRef.Name
	if gwpName == "" {
		err := errors.New("parametersRef.name cannot be empty when parametersRef is specified")
		//nolint:sloglint // keep the existing log key; renaming it would break log queries
		logger.Error("could not get gateway parameters for gateway class",
			"error", err,
			"gatewayClassName", gwc.GetName(),
		)
		return nil, err
	}

	gwpNamespace := ""
	if paramRef.Namespace != nil {
		gwpNamespace = string(*paramRef.Namespace)
	}

	gwp := k.gwParamClient.Get(gwpName, gwpNamespace)
	if gwp == nil {
		return nil, deployer.GetGatewayParametersForGatewayClassError(
			ErrNotFound,
			gwpNamespace, gwpName,
			gwc.GetName(),
			"GatewayClass",
		)
	}

	return gwp, nil
}

// mergeWithDefaults generates in-memory defaults with the given
// omitDefaultSecurityContext flag, then merges the explicit GatewayParameters on
// top. This ensures image registry/tag defaults are present when not overridden.
func (k *kgatewayParameters) mergeWithDefaults(gwc *gwv1.GatewayClass, gwp *kgateway.GatewayParameters, omitDefaultSecurityContext bool) (*kgateway.GatewayParameters, error) {
	defaultGwp, err := deployer.GetInMemoryGatewayParameters(deployer.InMemoryGatewayParametersConfig{
		ControllerName:             string(gwc.Spec.ControllerName),
		ClassName:                  gwc.GetName(),
		ImageInfo:                  k.inputs.ImageInfo,
		OmitDefaultSecurityContext: omitDefaultSecurityContext,
	})
	if err != nil {
		return nil, err
	}

	deployer.DeepMergeGatewayParameters(defaultGwp, gwp)
	return defaultGwp, nil
}

func (k *kgatewayParameters) getValues(gw *gwv1.Gateway, gwParam *kgateway.GatewayParameters) (*deployer.HelmConfig, error) {
	irGW := deployer.GetGatewayIR(gw, k.inputs.CommonCollections)
	ports := deployer.GetPortsValues(irGW, gwParam)
	if len(ports) == 0 {
		return nil, ErrNoValidPorts
	}

	gtw := &deployer.HelmGateway{
		Name:             &gw.Name,
		FullnameOverride: &gw.Name,
		GatewayName:      &gw.Name,
		GatewayNamespace: &gw.Namespace,
		GatewayClassName: new(string(gw.Spec.GatewayClassName)),
		Ports:            ports,
		Xds: &deployer.HelmXds{
			// The xds host/port MUST map to the Service definition for the Control Plane
			// This is the socket address that the Proxy will connect to on startup, to receive xds updates
			Host: &k.inputs.ControlPlane.XdsHost,
			Port: &k.inputs.ControlPlane.XdsPort,
			Tls: &deployer.HelmXdsTls{
				Enabled: new(k.inputs.ControlPlane.XdsTLS),
				CaCert:  new(k.inputs.ControlPlane.XdsTlsCaPath),
			},
		},
	}
	// The proxy's own listeners (readiness, stats) must bind the same address
	// family as the translated data-plane listeners; otherwise the kubelet probe
	// and the metrics scrape reach the pod on an address Envoy is not listening on.
	gtw.BindIpv6 = new(k.inputs.CommonCollections.Settings.ListenerBindIpv6)
	deployer.SetInfrastructureMetadataFromGateway(gw, gtw)
	// construct the default values
	vals := &deployer.HelmConfig{
		Gateway: gtw,
	}

	// Inject xDS CA certificate into Helm values if TLS is enabled
	if k.inputs.ControlPlane.XdsTLS {
		if err := injectXdsCACertificate(k.inputs.ControlPlane.XdsTlsCaPath, vals); err != nil {
			return nil, fmt.Errorf("failed to inject xDS CA certificate: %w", err)
		}
	}

	// if there is no GatewayParameters, return the values as is
	if gwParam == nil {
		return vals, nil
	}

	// extract all the custom values from the GatewayParameters
	if err := deployer.ApplyKubernetesProxyConfigValues(vals.Gateway, gwParam.Spec.Kube, k.inputs.IstioAutoMtlsEnabled); err != nil {
		return nil, err
	}

	// Extract loadBalancerIP from Gateway.spec.addresses and set it on the service if service type is LoadBalancer
	if err := deployer.SetLoadBalancerIPFromGateway(gw, vals.Gateway.Service); err != nil {
		return nil, err
	}

	return vals, nil
}

// resolvedKgatewayParameters holds the resolved parameters for a Gateway, supporting
// both GatewayClass-level and Gateway-level GatewayParameters for overlay application.
type resolvedKgatewayParameters struct {
	// gatewayClassGWP is the GatewayParameters from the GatewayClass (if any).
	gatewayClassGWP *kgateway.GatewayParameters
	// gatewayGWP is the GatewayParameters from the Gateway (if any).
	gatewayGWP *kgateway.GatewayParameters
}

// resolveParametersForOverlays resolves the GatewayParameters for the Gateway.
// It returns both GatewayClass-level and Gateway-level parameters separately
// to support ordered overlay merging (GatewayClass first, then Gateway).
// Unlike getGatewayParametersForGateway, this does NOT merge the parameters.
func (k *kgatewayParameters) resolveParametersForOverlays(gw *gwv1.Gateway) *resolvedKgatewayParameters {
	result := &resolvedKgatewayParameters{}

	// Get GatewayClass parameters first
	gwc := k.gwClassClient.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc != nil && gwc.Spec.ParametersRef != nil {
		ref := gwc.Spec.ParametersRef

		// Check for GatewayParameters on GatewayClass
		if ref.Group == kgateway.GroupName && string(ref.Kind) == wellknown.GatewayParametersGVK.Kind {
			gwpNamespace := ""
			if ref.Namespace != nil {
				gwpNamespace = string(*ref.Namespace)
			}
			gwp := k.gwParamClient.Get(ref.Name, gwpNamespace)
			if gwp != nil {
				result.gatewayClassGWP = gwp
			}
		}
	}

	// Check if Gateway has its own parametersRef
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil {
		ref := gw.Spec.Infrastructure.ParametersRef

		if ref.Group == kgateway.GroupName && ref.Kind == gwv1.Kind(wellknown.GatewayParametersGVK.Kind) {
			gwp := k.gwParamClient.Get(ref.Name, gw.GetNamespace())
			if gwp != nil {
				result.gatewayGWP = gwp
			}
		}
	}

	return result
}

func getGatewayClassFromGateway(cli kclient.Client[*gwv1.GatewayClass], gw *gwv1.Gateway) (*gwv1.GatewayClass, error) {
	if gw == nil {
		return nil, errors.New("nil Gateway")
	}
	if gw.Spec.GatewayClassName == "" {
		return nil, errors.New("GatewayClassName must not be empty")
	}

	gwc := cli.Get(string(gw.Spec.GatewayClassName), metav1.NamespaceNone)
	if gwc == nil {
		return nil, fmt.Errorf("failed to get GatewayClass for Gateway %s/%s", gw.GetName(), gw.GetNamespace())
	}

	return gwc, nil
}

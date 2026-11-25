package deployer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const gatewayParamsName = "gw-params"

// bootstrapTestCase is the GatewayParameters a Gateway's objects are rendered from.
type bootstrapTestCase struct {
	// classParams is the GatewayClass's GatewayParameters; nil for none.
	classParams *kgateway.GatewayParameters
	// gatewayParams is the Gateway's GatewayParameters; nil for none.
	gatewayParams *kgateway.GatewayParameters
	// listenerPorts are the Gateway's listener ports; defaults to 8080.
	listenerPorts []gwv1.PortNumber
	// validator validates the merged bootstrap; nil for none.
	validator validator.BootstrapValidator
	// managedPaths is the KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS mode; empty means WARN.
	managedPaths apisettings.BootstrapOverlayManagedPathsMode
	// override builds the override HelmValuesGenerator from kgateway's own; nil for none.
	override func(kgatewayValues deployer.HelmValuesGenerator) deployer.HelmValuesGenerator
}

// render renders the Gateway's objects through the full deployer, including post-processing.
func (tc bootstrapTestCase) render(t *testing.T) ([]client.Object, *deployer.PostProcessReport, error) {
	t.Helper()

	// Build the GatewayClass, the Gateway and their GatewayParameters.
	gwc := defaultGatewayClass()
	classParams := tc.classParams
	if classParams == nil {
		classParams = emptyGatewayParameters()
	}
	ports := tc.listenerPorts
	if len(ports) == 0 {
		ports = []gwv1.PortNumber{8080}
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: defaultNamespace, UID: "1235", Generation: 3},
		Spec:       gwv1.GatewaySpec{GatewayClassName: wellknown.DefaultGatewayClassName},
	}
	for _, p := range ports {
		gw.Spec.Listeners = append(gw.Spec.Listeners, gwv1.Listener{Name: gwv1.SectionName(fmt.Sprintf("port-%d", p)), Protocol: gwv1.HTTPProtocolType, Port: p})
	}
	objs := []client.Object{gwc, classParams}
	if tc.gatewayParams != nil {
		gw.Spec.Infrastructure = &gwv1.GatewayInfrastructure{ParametersRef: &gwv1.LocalParametersReference{
			Group: kgateway.GroupName,
			Kind:  gwv1.Kind(wellknown.GatewayParametersGVK.Kind),
			Name:  tc.gatewayParams.Name,
		}}
		objs = append(objs, tc.gatewayParams)
	}

	// Render through the deployer.
	fakeClient := fake.NewClient(t, objs...)
	inputs := defaultInputs(t, gwc, gw)
	inputs.BootstrapValidator = tc.validator
	inputs.BootstrapOverlayManagedPaths = tc.managedPaths
	gwp := NewGatewayParameters(fakeClient, inputs)
	if tc.override != nil {
		gwp.WithHelmValuesGeneratorOverride(tc.override(gwp.kgwParameters))
	}
	d, err := NewGatewayDeployer(wellknown.DefaultGatewayControllerName, schemes.GatewayScheme(), fakeClient, gwp)
	require.NoError(t, err)
	fakeClient.RunAndWait(t.Context().Done())
	return d.GetObjsToDeployWithReport(t.Context(), gw)
}

// withOverlay returns GatewayParameters whose bootstrap overlay is the given YAML.
func withOverlay(t *testing.T, gwp *kgateway.GatewayParameters, overlayYAML string) *kgateway.GatewayParameters {
	t.Helper()
	raw, err := yaml.YAMLToJSON([]byte(overlayYAML))
	require.NoError(t, err)
	if gwp.Spec.Kube == nil {
		gwp.Spec.Kube = &kgateway.KubernetesProxyConfig{}
	}
	gwp.Spec.Kube.EnvoyContainer = &kgateway.EnvoyContainer{Bootstrap: &kgateway.EnvoyBootstrap{
		Overlay: &kgateway.BootstrapOverlay{
			MergeStrategy: kgateway.BootstrapMergeStrategyStructuredMerge,
			Value:         apiextensionsv1.JSON{Raw: raw},
		},
	}}
	return gwp
}

// testOverlay returns the given YAML as an overlay from the Gateway's GatewayParameters.
func testOverlay(t *testing.T, overlayYAML string) bootstrapoverlay.Overlay {
	t.Helper()
	raw, err := yaml.YAMLToJSON([]byte(overlayYAML))
	require.NoError(t, err)
	return bootstrapoverlay.Overlay{Source: gatewayParamsSource(), Raw: raw}
}

func gatewayParams() *kgateway.GatewayParameters {
	return &kgateway.GatewayParameters{ObjectMeta: metav1.ObjectMeta{Name: gatewayParamsName, Namespace: defaultNamespace}}
}

func objectsOf[T client.Object](objs []client.Object) []T {
	var out []T
	for _, o := range objs {
		if typed, ok := o.(T); ok {
			out = append(out, typed)
		}
	}
	return out
}

// renderedBootstrap parses the bootstrap in the rendered ConfigMap.
func renderedBootstrap(t *testing.T, objs []client.Object) (string, *envoybootstrapv3.Bootstrap) {
	t.Helper()
	cm := findBootstrapConfigMap(objs)
	require.NotNil(t, cm, "no bootstrap ConfigMap rendered")
	bs, err := bootstrapoverlay.ParseBootstrap([]byte(cm.Data[bootstrapConfigKey]))
	require.NoError(t, err)
	return cm.Data[bootstrapConfigKey], bs
}

func checksum(t *testing.T, objs []client.Object) string {
	t.Helper()
	return podTemplateAnnotations(t, objs)[bootstrapChecksumAnnotation]
}

func podTemplateAnnotations(t *testing.T, objs []client.Object) map[string]string {
	t.Helper()
	deps := objectsOf[*appsv1.Deployment](objs)
	require.Len(t, deps, 1)
	return deps[0].Spec.Template.Annotations
}

func bootstrapCondition(t *testing.T, report *deployer.PostProcessReport) metav1.Condition {
	t.Helper()
	require.NotNil(t, report)
	assert.Equal(t, []string{conditions.KgatewayConditionBootstrapOverlayApplied}, report.OwnedConditionTypes)
	require.Len(t, report.Conditions, 1)
	return report.Conditions[0]
}

func TestBootstrapOverlaysNotSet(t *testing.T) {
	// Render a Gateway whose GatewayParameters have no bootstrap overlays.
	objs, report, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
	require.NoError(t, err)

	// The bootstrap is the Helm-rendered text, byte for byte, with Helm's checksum.
	envoyYaml, _ := renderedBootstrap(t, objs)
	sum := sha256.Sum256([]byte(envoyYaml))
	assert.Equal(t, hex.EncodeToString(sum[:]), checksum(t, objs), "checksum should be Helm's SHA-256 of the rendered bootstrap")
	assert.Contains(t, envoyYaml, "port_value: 19000\n", "the bootstrap should keep the Helm template's formatting")

	// Without overlays the translator validates against Envoy's defaults, so there is no
	// validation data to write.
	assert.NotContains(t, findBootstrapConfigMap(objs).Data, bootstrapoverlay.ValidationKey)
	assert.NotContains(t, podTemplateAnnotations(t, objs), bootstrapoverlay.ValidationAnnotation)

	// The report clears the condition, sets none, and records no events.
	require.NotNil(t, report)
	assert.Empty(t, report.Conditions)
	assert.Equal(t, []string{conditions.KgatewayConditionBootstrapOverlayApplied}, report.OwnedConditionTypes)
	assert.Empty(t, report.Events)
}

func TestBootstrapOverlaysApplied(t *testing.T) {
	// Render a Gateway whose GatewayParameters add a runtime layer.
	overlay := `
layered_runtime:
  layers:
  - name: user_runtime
    static_layer: {envoy.reloadable_features.http_reject_path_with_fragment: false}`
	objs, report, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)
	require.NoError(t, err)

	// The layer is merged before the admin layer.
	envoyYaml, bs := renderedBootstrap(t, objs)
	var layerNames []string
	for _, l := range bs.GetLayeredRuntime().GetLayers() {
		layerNames = append(layerNames, l.GetName())
	}
	assert.Equal(t, []string{"user_runtime", "admin_layer"}, layerNames)

	// The checksum matches the merged bootstrap.
	sum := sha256.Sum256([]byte(envoyYaml))
	assert.Equal(t, hex.EncodeToString(sum[:]), checksum(t, objs))

	// The condition reports the overlays as applied.
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapOverlayApplied, cond.Reason)
	assert.Equal(t, int64(3), cond.ObservedGeneration)
	assert.Equal(t, "The bootstrap overlay is applied.", cond.Message)
	assert.Empty(t, report.Events, "overlays with nothing to warn about should record no events")

	// Rendering the same overlays again gives the same checksum; changing them changes it.
	again, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)
	require.NoError(t, err)
	assert.Equal(t, checksum(t, objs), checksum(t, again), "identical overlays should give an identical checksum")
	changed, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	assert.NotEqual(t, checksum(t, objs), checksum(t, changed), "different overlays should change the checksum")
}

func TestBootstrapOverlaysValidationData(t *testing.T) {
	// Render a Gateway whose overlays add a static runtime layer and a static listener,
	// and move the readiness listener from 8082 to 8083.
	overlay := `
layered_runtime:
  layers:
  - name: limits
    static_layer: {re2.max_program_size.error_level: 1000}
static_resources:
  listeners:
  - name: user_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 9000}}
  - name: readiness_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 8083}}`
	objs, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)
	require.NoError(t, err)

	// The ConfigMap holds the static layer, and exactly the ports the merged bootstrap
	// listens on: 8082 is no longer one of them.
	data, err := bootstrapoverlay.DecodeValidationData(findBootstrapConfigMap(objs).Data[bootstrapoverlay.ValidationKey])
	require.NoError(t, err)
	assert.JSONEq(t, `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":1000}}]}}`, string(data.Bootstrap))
	assert.Equal(t, []uint32{8083, 9000, 9091, 19000}, data.ReservedPorts)

	// The pod template carries the same validation data, byte for byte, so the
	// first-snapshot hold can compare a pod's subset with the Gateway's.
	assert.Equal(t, findBootstrapConfigMap(objs).Data[bootstrapoverlay.ValidationKey], podTemplateAnnotations(t, objs)[bootstrapoverlay.ValidationAnnotation])

	// Overlays without a static layer write the empty subset, which the translator
	// treats as Envoy's defaults.
	objs, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	annotation, err := bootstrapoverlay.DecodeValidationData(podTemplateAnnotations(t, objs)[bootstrapoverlay.ValidationAnnotation])
	require.NoError(t, err)
	assert.Equal(t, bootstrapoverlay.EmptyValidationSubset, string(annotation.Bootstrap))
}

func TestBootstrapOverlaysValidationSubsetTooLarge(t *testing.T) {
	// Render a Gateway whose overlays set a static runtime layer larger than the
	// annotation limit.
	overlay := fmt.Sprintf(`{layered_runtime: {layers: [{name: big, static_layer: {key: %q}}]}}`,
		strings.Repeat("x", bootstrapoverlay.MaxValidationSubsetSize))
	objs, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)

	// Nothing is rendered, and the error names the limit.
	var overlayErr *BootstrapOverlayError
	require.ErrorAs(t, err, &overlayErr)
	assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
	assert.ErrorContains(t, overlayErr, "the static runtime layers of the merged bootstrap take")
	assert.Nil(t, objs)
}

func TestBootstrapOverlaysGatewayClassThenGateway(t *testing.T) {
	// Render a Gateway whose GatewayClass and Gateway both set the flush interval.
	objs, report, err := bootstrapTestCase{
		classParams:   withOverlay(t, emptyGatewayParameters(), `{stats_flush_interval: 10s, layered_runtime: {layers: [{name: class_layer, static_layer: {}}]}}`),
		gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 20s, layered_runtime: {layers: [{name: gateway_layer, static_layer: {}}]}}`),
	}.render(t)
	require.NoError(t, err)

	// The Gateway's value wins, and both layers apply in order.
	_, bs := renderedBootstrap(t, objs)
	assert.Equal(t, int64(20), bs.GetStatsFlushInterval().GetSeconds())
	var layerNames []string
	for _, l := range bs.GetLayeredRuntime().GetLayers() {
		layerNames = append(layerNames, l.GetName())
	}
	assert.Equal(t, []string{"class_layer", "gateway_layer", "admin_layer"}, layerNames)
	assert.Equal(t, metav1.ConditionTrue, bootstrapCondition(t, report).Status)
}

// postProcessingOverride is an override HelmValuesGenerator with its own post-processor.
// It renders kgateway's own values, records the bootstrap its post-processor is given,
// and labels the Deployment.
type postProcessingOverride struct {
	deployer.HelmValuesGenerator
	seenBootstrap string
}

func (o *postProcessingOverride) PostProcessObjects(_ context.Context, _ client.Object, rendered []client.Object) ([]client.Object, error) {
	// Record the bootstrap the post-processor sees.
	if cm := findBootstrapConfigMap(rendered); cm != nil {
		o.seenBootstrap = cm.Data[bootstrapConfigKey]
	}

	// Change an object, as a post-processor would.
	for _, dep := range objectsOf[*appsv1.Deployment](rendered) {
		if dep.Labels == nil {
			dep.Labels = map[string]string{}
		}
		dep.Labels["post-processed"] = "true"
	}
	return rendered, nil
}

// overlayProvidingOverride is a postProcessingOverride that also chooses the bootstrap
// overlays, and records the GatewayParameters overlays it is given.
type overlayProvidingOverride struct {
	postProcessingOverride
	overlays              func(fromGatewayParameters []bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error)
	fromGatewayParameters []bootstrapoverlay.Overlay
}

func (o *overlayProvidingOverride) BootstrapOverlays(_ context.Context, _ *gwv1.Gateway, fromGatewayParameters []bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error) {
	o.fromGatewayParameters = fromGatewayParameters
	return o.overlays(fromGatewayParameters)
}

// runtimeLayerNames returns the names of the bootstrap's runtime layers, in order.
func runtimeLayerNames(bs *envoybootstrapv3.Bootstrap) []string {
	var names []string
	for _, l := range bs.GetLayeredRuntime().GetLayers() {
		names = append(names, l.GetName())
	}
	return names
}

func TestBootstrapOverlaysOverridePostProcessor(t *testing.T) {
	// Render a Gateway with bootstrap overlays through an override that post-processes
	// the objects without choosing bootstrap overlays.
	override := &postProcessingOverride{}
	objs, report, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{layered_runtime: {layers: [{name: user_runtime, static_layer: {}}]}}`),
		override: func(kgatewayValues deployer.HelmValuesGenerator) deployer.HelmValuesGenerator {
			override.HelmValuesGenerator = kgatewayValues
			return override
		},
	}.render(t)
	require.NoError(t, err)

	// The override's post-processor ran.
	deps := objectsOf[*appsv1.Deployment](objs)
	require.Len(t, deps, 1)
	assert.Equal(t, "true", deps[0].Labels["post-processed"])

	// It replaced the bootstrap overlays: none are merged, and no condition is
	// reported; the report only removes one an earlier controller left.
	envoyYaml, bs := renderedBootstrap(t, objs)
	assert.Equal(t, []string{"admin_layer"}, runtimeLayerNames(bs))
	assert.Equal(t, &deployer.PostProcessReport{OwnedConditionTypes: []string{conditions.KgatewayConditionBootstrapOverlayApplied}}, report)

	// The generated bootstrap is still written as its own revision, after the
	// post-processor, and the proxy reads it.
	key := bootstrapRevisionKey(checksum(t, objs))
	assert.Equal(t, envoyYaml, findBootstrapConfigMap(objs).Data[key])
	assert.Equal(t, "/etc/envoy/"+key, proxyEnv(t, objs, inputConfigEnv))
}

func TestBootstrapOverlayProvider(t *testing.T) {
	const gatewayOverlay = `{layered_runtime: {layers: [{name: user_runtime, static_layer: {}}]}}`

	// render renders a Gateway with bootstrap overlays through an overlay-providing
	// override whose overlays come from the given function.
	render := func(t *testing.T, overlays func([]bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error)) (*overlayProvidingOverride, []client.Object, *deployer.PostProcessReport, error) {
		override := &overlayProvidingOverride{overlays: overlays}
		objs, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), gatewayOverlay),
			override: func(kgatewayValues deployer.HelmValuesGenerator) deployer.HelmValuesGenerator {
				override.HelmValuesGenerator = kgatewayValues
				return override
			},
		}.render(t)
		return override, objs, report, err
	}

	t.Run("the GatewayParameters overlays, unchanged", func(t *testing.T) {
		// Return the GatewayParameters overlays as given.
		override, objs, report, err := render(t, func(from []bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error) {
			return from, nil
		})
		require.NoError(t, err)

		// The override was given the Gateway's GatewayParameters overlay.
		require.Len(t, override.fromGatewayParameters, 1)
		want := testOverlay(t, gatewayOverlay)
		assert.Equal(t, want.Source, override.fromGatewayParameters[0].Source)
		assert.JSONEq(t, string(want.Raw), string(override.fromGatewayParameters[0].Raw))

		// The overlays are merged, and the post-processor ran on the merged bootstrap.
		envoyYaml, bs := renderedBootstrap(t, objs)
		assert.Equal(t, []string{"user_runtime", "admin_layer"}, runtimeLayerNames(bs))
		assert.Equal(t, envoyYaml, override.seenBootstrap, "the post-processor should see the merged bootstrap")
		assert.Equal(t, "true", objectsOf[*appsv1.Deployment](objs)[0].Labels["post-processed"])

		// The condition reports the overlays as applied.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonBootstrapOverlayApplied, cond.Reason)
	})

	t.Run("the override's own overlays", func(t *testing.T) {
		// Return an overlay from another kind of resource in place of the GatewayParameters
		// overlay. It also sets a managed path, so the condition names its source.
		own := testOverlay(t, `{node: {id: x}, layered_runtime: {layers: [{name: provider_runtime, static_layer: {}}]}}`)
		own.Source = bootstrapoverlay.Source{Kind: "ProxyParameters", Namespace: defaultNamespace, Name: "provider-params"}
		_, objs, report, err := render(t, func([]bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error) {
			return []bootstrapoverlay.Overlay{own}, nil
		})
		require.NoError(t, err)

		// Only the override's overlay is merged.
		_, bs := renderedBootstrap(t, objs)
		assert.Equal(t, []string{"provider_runtime", "admin_layer"}, runtimeLayerNames(bs))

		// The condition names the override's resource by its kind.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonBootstrapManagedPathsOverridden, cond.Reason)
		assert.Contains(t, cond.Message, "node (ProxyParameters default/provider-params)")
	})

	t.Run("no overlays", func(t *testing.T) {
		// Return no overlays.
		_, objs, report, err := render(t, func([]bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error) {
			return nil, nil
		})
		require.NoError(t, err)

		// Nothing is merged, and the report clears the condition.
		_, bs := renderedBootstrap(t, objs)
		assert.Equal(t, []string{"admin_layer"}, runtimeLayerNames(bs))
		require.NotNil(t, report)
		assert.Empty(t, report.Conditions)
		assert.Equal(t, []string{conditions.KgatewayConditionBootstrapOverlayApplied}, report.OwnedConditionTypes)
	})

	t.Run("an error", func(t *testing.T) {
		// Fail to choose the overlays.
		_, objs, _, err := render(t, func([]bootstrapoverlay.Overlay) ([]bootstrapoverlay.Overlay, error) {
			return nil, errors.New("the provider's parameters are missing")
		})

		// Nothing is rendered, and the error names the Gateway and the cause.
		assert.ErrorContains(t, err, fmt.Sprintf("failed to get the bootstrap overlay for Gateway %s/gw: the provider's parameters are missing", defaultNamespace))
		assert.Nil(t, objs)
	})
}

func TestBootstrapOverlaysManagedPath(t *testing.T) {
	// Render a Gateway whose overlays replace the xDS cluster.
	objs, report, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster, connect_timeout: 9s}]}}`),
	}.render(t)
	require.NoError(t, err)

	// Under WARN the replacement applies and the condition names the path and its GatewayParameters.
	_, bs := renderedBootstrap(t, objs)
	for _, c := range bs.GetStaticResources().GetClusters() {
		if c.GetName() == "xds_cluster" {
			assert.Equal(t, int64(9), c.GetConnectTimeout().GetSeconds())
		}
	}
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapManagedPathsOverridden, cond.Reason)
	assert.Contains(t, cond.Message, "static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params)")
}

func TestBootstrapOverlaysGeneratedEntryManaged(t *testing.T) {
	// Render a Gateway whose overlays replace the downstream-connections resource
	// monitor that maxDownstreamConnections renders: every named entry of the
	// generated bootstrap is a managed path.
	gwp := withOverlay(t, gatewayParams(), `
overload_manager:
  resource_monitors:
  - name: envoy.resource_monitors.global_downstream_max_connections
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.resource_monitors.downstream_connections.v3.DownstreamConnectionsConfig
      max_active_downstream_connections: 500`)
	gwp.Spec.Kube.EnvoyContainer.Bootstrap.MaxDownstreamConnections = new(int64(100))
	objs, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
	require.NoError(t, err)

	// Under WARN the replacement applies.
	_, bs := renderedBootstrap(t, objs)
	monitors := bs.GetOverloadManager().GetResourceMonitors()
	require.Len(t, monitors, 1, "the overlay should replace the generated monitor, not add a second")
	assert.Contains(t, monitors[0].GetTypedConfig().String(), "500")

	// The condition and a Warning event name the entry as a managed path.
	const path = "overload_manager.resource_monitors[name=envoy.resource_monitors.global_downstream_max_connections]"
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapManagedPathsOverridden, cond.Reason)
	assert.Contains(t, cond.Message, path+" (GatewayParameters default/gw-params)")
	require.Len(t, report.Events, 1)
	assert.Equal(t, conditions.KgatewayReasonBootstrapManagedPathsOverridden, report.Events[0].Reason)
	assert.Contains(t, report.Events[0].Note, path)
}

func TestBootstrapOverlaysStaticNamePrefix(t *testing.T) {
	// Render a Gateway whose overlays add a static cluster named like a translated
	// backend cluster.
	_, _, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: kube_default_httpbin_8000}]}}`),
	}.render(t)

	// Nothing is rendered, and the error names the prefix.
	var overlayErr *BootstrapOverlayError
	require.ErrorAs(t, err, &overlayErr)
	assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
	assert.ErrorContains(t, overlayErr, `static cluster "kube_default_httpbin_8000" is not in the generated bootstrap and its name does not start with "user_"`)

	// A cluster with the prefix applies.
	objs, _, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: user_cluster}]}}`),
	}.render(t)
	require.NoError(t, err)
	_, bs := renderedBootstrap(t, objs)
	assert.True(t, slices.ContainsFunc(bs.GetStaticResources().GetClusters(), func(c *envoyclusterv3.Cluster) bool { return c.GetName() == "user_cluster" }))
}

func TestBootstrapOverlaysInvalid(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		wantErr string
	}{
		{
			name:    "misspelled field",
			overlay: `{layerd_runtime: {}}`,
			wantErr: "bootstrap overlay in GatewayParameters default/gw-params: does not parse as an Envoy Bootstrap",
		},
		{
			name:    "runtime layer named like the admin layer",
			overlay: `{layered_runtime: {layers: [{name: admin_layer, static_layer: {}}]}}`,
			wantErr: `runtime layer "admin_layer" has the name of a layer in the generated bootstrap`,
		},
		{
			name:    "disk runtime layer",
			overlay: `{layered_runtime: {layers: [{name: from_disk, disk_layer: {symlink_root: /etc/runtime}}]}}`,
			wantErr: `runtime layer "from_disk" sets disk_layer instead of static_layer; a bootstrap overlay can add only static runtime layers`,
		},
		{
			name:    "RTDS runtime layer",
			overlay: `{layered_runtime: {layers: [{name: rt, rtds_layer: {name: rt, rtds_config: {ads: {}}}}]}}`,
			wantErr: `runtime layer "rt" sets rtds_layer instead of static_layer`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Render a Gateway with invalid overlays.
			objs, report, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), tt.overlay)}.render(t)

			// Nothing is rendered, and the error carries the condition reason.
			var overlayErr *BootstrapOverlayError
			require.ErrorAs(t, err, &overlayErr)
			assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
			assert.Equal(t, "gw", overlayErr.ConfigMapName)
			assert.ErrorContains(t, overlayErr, tt.wantErr)
			assert.Nil(t, objs)
			assert.Nil(t, report)
		})
	}
}

// fakeBootstrapValidator records the bootstrap it validates and returns a programmed response.
type fakeBootstrapValidator struct {
	got *envoybootstrapv3.Bootstrap
	err error
}

func (f *fakeBootstrapValidator) ValidateBootstrap(_ context.Context, bs *envoybootstrapv3.Bootstrap) error {
	f.got = bs
	return f.err
}

func TestBootstrapOverlaysValidation(t *testing.T) {
	gwp := func() *kgateway.GatewayParameters {
		return withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)
	}

	t.Run("accepted", func(t *testing.T) {
		// Render a Gateway whose merged bootstrap Envoy accepts.
		v := &fakeBootstrapValidator{}
		objs, report, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)
		require.NoError(t, err)

		// Envoy validated the merged bootstrap, which is what the ConfigMap holds.
		require.NotNil(t, v.got)
		assert.Equal(t, int64(10), v.got.GetStatsFlushInterval().GetSeconds())
		_, written := renderedBootstrap(t, objs)
		assert.True(t, proto.Equal(written, v.got), "the validated bootstrap should be the one written")

		// The overlays apply.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonBootstrapOverlayApplied, cond.Reason)
		assert.Equal(t, "The bootstrap overlay is applied.", cond.Message)
	})

	t.Run("rejected", func(t *testing.T) {
		// Render a Gateway whose merged bootstrap Envoy rejects.
		v := &fakeBootstrapValidator{err: fmt.Errorf("%w: error initializing configuration '': bad", validator.ErrInvalidBootstrap)}
		objs, report, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)

		// Nothing is rendered, and the error carries the reason and Envoy's message.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapValidationFailed, overlayErr.Reason)
		assert.Equal(t, "gw", overlayErr.ConfigMapName)
		assert.EqualError(t, overlayErr, "Envoy rejected the bootstrap: error initializing configuration '': bad")
		assert.Nil(t, objs)
		assert.Nil(t, report)
	})

	t.Run("Envoy did not finish", func(t *testing.T) {
		// Render a Gateway when Envoy is killed while validating.
		v := &fakeBootstrapValidator{err: errors.New("Envoy was killed while validating the bootstrap")}
		_, _, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)

		// The overlays are reported as not validated, and the reconcile retries.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapValidationUnavailable, overlayErr.Reason)
		assert.True(t, overlayErr.Retry, "validating again can succeed")
		assert.EqualError(t, overlayErr, "could not validate the merged bootstrap: Envoy was killed while validating the bootstrap; the controller retries")
	})

	t.Run("the controller cannot run Envoy", func(t *testing.T) {
		// Render a Gateway when the Envoy binary is missing.
		v := &fakeBootstrapValidator{err: fmt.Errorf("%w: exec: no such file", validator.ErrBootstrapValidatorUnavailable)}
		_, _, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)

		// The overlays are reported as not validated, without a retry: the restart that
		// a change to the controller's Deployment causes reconciles the Gateway again.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapValidationUnavailable, overlayErr.Reason)
		assert.False(t, overlayErr.Retry)
		assert.EqualError(t, overlayErr, "the controller cannot run Envoy to validate the bootstrap: exec: no such file; the controller retries after its Deployment or settings change")
	})

	t.Run("the controller is stopping", func(t *testing.T) {
		// Render a Gateway's objects without overlays, then apply overlays with a context
		// that has ended.
		objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		v := &fakeBootstrapValidator{err: context.Canceled}
		_, err = ApplyBootstrapOverlays(ctx, objs, []bootstrapoverlay.Overlay{testOverlay(t, `{stats_flush_interval: 10s}`)}, BootstrapOverlayConfig{Validator: v})

		// The error is the context's, not a report on the overlays.
		require.ErrorIs(t, err, context.Canceled)
		var overlayErr *BootstrapOverlayError
		assert.False(t, errors.As(err, &overlayErr))
	})

	t.Run("a controller failure", func(t *testing.T) {
		// Apply overlays to rendered objects that have no bootstrap ConfigMap.
		_, err := ApplyBootstrapOverlays(t.Context(), nil, []bootstrapoverlay.Overlay{testOverlay(t, `{stats_flush_interval: 10s}`)}, BootstrapOverlayConfig{})

		// The failure is reported as the controller's, without a retry or a ConfigMap name.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapControllerError, overlayErr.Reason)
		assert.False(t, overlayErr.Retry)
		assert.Empty(t, overlayErr.ConfigMapName)
		assert.EqualError(t, overlayErr, "the controller failed to apply the bootstrap overlay; this is a kgateway defect, not a problem with the overlay: no bootstrap ConfigMap was rendered")
	})

	t.Run("not run without overlays", func(t *testing.T) {
		v := &fakeBootstrapValidator{}
		_, _, err := bootstrapTestCase{gatewayParams: gatewayParams(), validator: v}.render(t)
		require.NoError(t, err)
		assert.Nil(t, v.got)
	})
}

func TestBootstrapOverlaysListenerPortCollision(t *testing.T) {
	// Render a Gateway with listeners on 8080 and 8443, whose overlays add a static
	// listener on 8443.
	overlay := `
static_resources:
  listeners:
  - name: user_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 8443}}`
	objs, report, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), overlay),
		listenerPorts: []gwv1.PortNumber{8080, 8443},
	}.render(t)
	require.NoError(t, err)

	// The colliding port is left off the Service, and the condition says so.
	svcs := objectsOf[*corev1.Service](objs)
	require.Len(t, svcs, 1)
	var svcPorts []int32
	for _, p := range svcs[0].Spec.Ports {
		svcPorts = append(svcPorts, p.Port)
	}
	assert.Equal(t, []int32{8080}, svcPorts)
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Contains(t, cond.Message, "uses Gateway listener port(s) 8443")

	// When every listener port collides, nothing is deployed, and the error, which the
	// condition reports, names the ports.
	objs, _, err = bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), overlay),
		listenerPorts: []gwv1.PortNumber{8443},
	}.render(t)
	assert.Nil(t, objs)
	overlayErr, ok := errors.AsType[*BootstrapOverlayError](err)
	require.True(t, ok, "expected a *BootstrapOverlayError, got %v", err)
	assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
	assert.NotEmpty(t, overlayErr.ConfigMapName, "an existing Gateway's Accepted condition should be left alone")
	assert.ErrorContains(t, overlayErr, "the merged bootstrap listens on every Gateway listener port (8443)")
}

func TestBootstrapOverlaysDefaultListenerPorts(t *testing.T) {
	// Render a Gateway with listeners on 8080 and 8082, whose overlays leave the
	// readiness listener on 8082.
	serviceOf := func(objs []client.Object) []int32 {
		svcs := objectsOf[*corev1.Service](objs)
		require.Len(t, svcs, 1)
		var ports []int32
		for _, p := range svcs[0].Spec.Ports {
			ports = append(ports, p.Port)
		}
		return ports
	}
	objs, report, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`),
		listenerPorts: []gwv1.PortNumber{8080, 8082},
	}.render(t)
	require.NoError(t, err)

	// 8082 is left off the Service, as without overlays. The translator rejects the
	// listener on it, so the condition need not name it.
	assert.Equal(t, []int32{8080}, serviceOf(objs))
	assert.NotContains(t, bootstrapCondition(t, report).Message, "Gateway listener port")

	// The default ports stay reserved when overlays move the readiness listener to
	// 8083, so 8082 is still left off the Service.
	moved := `
static_resources:
  listeners:
  - name: readiness_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 8083}}`
	objs, _, err = bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), moved),
		listenerPorts: []gwv1.PortNumber{8080, 8082},
	}.render(t)
	require.NoError(t, err)
	assert.Equal(t, []int32{8080}, serviceOf(objs))
}

func TestBootstrapOverlaysNotMounted(t *testing.T) {
	for name, tc := range map[string]struct {
		deploymentOverlay string
		message           string
	}{
		"another ConfigMap in the bootstrap volume": {
			deploymentOverlay: `{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`,
			message:           `a deploymentOverlay mounts ConfigMap "my-bootstrap" at /etc/envoy instead of the generated ConfigMap "gw"`,
		},
		"another volume at /etc/envoy": {
			deploymentOverlay: `{"template": {"spec": {
				"volumes": [{"name": "my-volume", "emptyDir": {}}],
				"containers": [{"name": "kgateway-proxy", "volumeMounts": [{"mountPath": "/etc/envoy", "name": "my-volume"}]}]}}}`,
			message: `a deploymentOverlay mounts volume "my-volume" at /etc/envoy, which is not the generated ConfigMap "gw"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Render a Gateway whose overlays apply but whose deploymentOverlay mounts
			// something else at /etc/envoy.
			gwp := withOverlay(t, gatewayParams(), `{layered_runtime: {layers: [{name: limits, static_layer: {re2.max_program_size.error_level: 1000}}]}}`)
			gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(tc.deploymentOverlay)}}
			objs, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
			require.NoError(t, err)

			// The objects deploy as the overlay asks, and the condition says the overlays
			// are not in effect.
			cond := bootstrapCondition(t, report)
			assert.Equal(t, metav1.ConditionFalse, cond.Status)
			assert.Equal(t, conditions.KgatewayReasonBootstrapNotMounted, cond.Reason)
			assert.Equal(t, "The bootstrap overlay is not in effect: "+tc.message+". Routes are validated against Envoy's defaults, not the overlay.", cond.Message)

			// The proxy does not run the overlays, so there is no validation data, no
			// revision key and no INPUT_CONF, and routes validate against Envoy's defaults.
			cm := findBootstrapConfigMap(objs)
			assert.NotContains(t, cm.Data, bootstrapoverlay.ValidationKey)
			assert.NotContains(t, cm.Data, bootstrapRevisionKey(checksum(t, objs)))
			assert.NotContains(t, podTemplateAnnotations(t, objs), bootstrapoverlay.ValidationAnnotation)
			assert.Empty(t, proxyEnv(t, objs, inputConfigEnv))
		})
	}

	t.Run("Service ports", func(t *testing.T) {
		// Render a Gateway with listeners on 8080, 8082 and 8443, whose overlays move the
		// readiness listener off 8082 and add a static listener on 8443, and whose
		// deploymentOverlay mounts another ConfigMap at /etc/envoy.
		gwp := withOverlay(t, gatewayParams(), `{static_resources: {listeners: [
			{name: readiness_listener, address: {socket_address: {address: 0.0.0.0, port_value: 8083}}},
			{name: user_listener, address: {socket_address: {address: 0.0.0.0, port_value: 8443}}}]}}`)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`)}}
		objs, report, err := bootstrapTestCase{gatewayParams: gwp, listenerPorts: []gwv1.PortNumber{8080, 8082, 8443}}.render(t)
		require.NoError(t, err)

		// The proxy runs a bootstrap the controller cannot read, and the translator
		// reserves the default ports, so the Service is built as for a Gateway without
		// overlays: the default port 8082 is left off, and 8443 is not.
		assert.Equal(t, []int32{8080, 8443}, servicePorts(t, objs))
		cond := bootstrapCondition(t, report)
		assert.Equal(t, conditions.KgatewayReasonBootstrapNotMounted, cond.Reason)
		assert.NotContains(t, cond.Message, "Service", "the Service is built as for a Gateway without overlays, so the condition need not mention it")
	})

	t.Run("INPUT_CONF set in the GatewayParameters", func(t *testing.T) {
		// Render a Gateway whose overlay applies but whose GatewayParameters point the
		// proxy at another bootstrap file.
		userEnv := corev1.EnvVar{Name: inputConfigEnv, Value: "/custom/envoy.yaml"}
		gwp := withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)
		gwp.Spec.Kube.EnvoyContainer.Env = []corev1.EnvVar{userEnv}
		objs, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
		require.NoError(t, err)

		// The condition says the overlay is not in effect, and names the variable.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonBootstrapNotMounted, cond.Reason)
		assert.Equal(t, `The bootstrap overlay is not in effect: the kgateway-proxy container's env sets INPUT_CONF to a path outside /etc/envoy, so the proxy reads its bootstrap from that file and not from the generated ConfigMap "gw". Routes are validated against Envoy's defaults, not the overlay.`, cond.Message)

		// The proxy keeps the path the GatewayParameters set, and gets no revision key
		// or validation data.
		cm := findBootstrapConfigMap(objs)
		assert.Equal(t, userEnv.Value, proxyEnv(t, objs, inputConfigEnv))
		assert.NotContains(t, cm.Data, bootstrapRevisionKey(checksum(t, objs)))
		assert.NotContains(t, cm.Data, bootstrapoverlay.ValidationKey)

		// Render the same Gateway without an overlay.
		gwp = gatewayParams()
		gwp.Spec.Kube = &kgateway.KubernetesProxyConfig{EnvoyContainer: &kgateway.EnvoyContainer{Env: []corev1.EnvVar{userEnv}}}
		objs, report, err = bootstrapTestCase{gatewayParams: gwp}.render(t)
		require.NoError(t, err)

		// The proxy keeps its path and gets no revision key, and nothing is reported.
		assert.Equal(t, userEnv.Value, proxyEnv(t, objs, inputConfigEnv))
		assert.Empty(t, revisionKeys(findBootstrapConfigMap(objs)), "a proxy that reads another file should have no revision key")
		assert.Empty(t, report.Conditions, "a Gateway without an overlay has no condition")

		// Render it with INPUT_CONF naming the generated ConfigMap's envoy.yaml.
		gwp.Spec.Kube.EnvoyContainer.Env = []corev1.EnvVar{{Name: inputConfigEnv, Value: "/etc/envoy/envoy.yaml"}}
		objs, _, err = bootstrapTestCase{gatewayParams: gwp}.render(t)
		require.NoError(t, err)

		// That file is the controller's, so the proxy is pointed at its revision key.
		assert.Equal(t, "/etc/envoy/envoy-"+checksum(t, objs)+".yaml", proxyEnv(t, objs, inputConfigEnv))
	})

	t.Run("invalid overlay", func(t *testing.T) {
		// Render a Gateway with an invalid overlay whose deploymentOverlay mounts another
		// ConfigMap at /etc/envoy.
		gwp := withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`)}}
		_, _, err := bootstrapTestCase{gatewayParams: gwp}.render(t)

		// The overlay's own failure is reported, and the error says the overlay could not
		// reach the proxy and how to deploy the held-back objects.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
		assert.Equal(t, `a deploymentOverlay mounts ConfigMap "my-bootstrap" at /etc/envoy instead of the generated ConfigMap "gw"`, overlayErr.MountProblem)
		assert.Equal(t, `The proxy does not read the generated bootstrap (a deploymentOverlay mounts ConfigMap "my-bootstrap" at /etc/envoy instead of the generated ConfigMap "gw"), so the overlay has no effect on it; remove the overlay, or fix it before removing that change, to deploy the Gateway's other changes.`, overlayErr.MountNote())

		// Render the same overlay with the generated bootstrap mounted.
		_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)}.render(t)

		// The overlay could reach the proxy, so there is no note.
		require.ErrorAs(t, err, &overlayErr)
		assert.Empty(t, overlayErr.MountProblem, "the proxy reads the generated bootstrap")
		assert.Empty(t, overlayErr.MountNote(), "no note when the proxy reads the generated bootstrap")
		assert.NoError(t, overlayErr.ObjectsErr, "the objects post-process")

		// Render the same overlay with a deploymentOverlay that does not apply.
		gwp = withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(`{"replicas": "three"}`)}}
		_, _, err = bootstrapTestCase{gatewayParams: gwp}.render(t)

		// The bootstrap overlay's failure is the one returned, and it carries the
		// deploymentOverlay's.
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
		assert.ErrorContains(t, overlayErr.ObjectsErr, "failed to apply overlay to Deployment/gw")
	})
}

func TestBootstrapMountProblem(t *testing.T) {
	// deploymentMounting returns a Deployment whose proxy container mounts volume at mountPath.
	deploymentMounting := func(volume corev1.Volume, mount corev1.VolumeMount) []client.Object {
		return []client.Object{&appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes:    []corev1.Volume{volume},
			Containers: []corev1.Container{{Name: wellknown.KgatewayContainerName, VolumeMounts: []corev1.VolumeMount{mount}}},
		}}}}}
	}
	generated := corev1.Volume{Name: "envoy-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: "gw"},
	}}}
	atEtcEnvoy := corev1.VolumeMount{Name: "envoy-config", MountPath: "/etc/envoy"}

	// The whole generated ConfigMap at /etc/envoy, with or without a trailing slash, is mounted.
	assert.Empty(t, BootstrapMountProblem(deploymentMounting(generated, atEtcEnvoy), "gw"))
	assert.Empty(t, BootstrapMountProblem(deploymentMounting(generated, corev1.VolumeMount{Name: "envoy-config", MountPath: "/etc/envoy/"}), "gw"))

	// A mount elsewhere, part of the ConfigMap, or some of its keys are not.
	assert.Equal(t, "a deploymentOverlay removes the kgateway-proxy container's mount at /etc/envoy",
		BootstrapMountProblem(deploymentMounting(generated, corev1.VolumeMount{Name: "envoy-config", MountPath: "/etc/other"}), "gw"))
	assert.Equal(t, `a deploymentOverlay mounts only part of the generated ConfigMap "gw" at /etc/envoy`,
		BootstrapMountProblem(deploymentMounting(generated, corev1.VolumeMount{Name: "envoy-config", MountPath: "/etc/envoy", SubPath: "envoy.yaml"}), "gw"))
	someKeys := *generated.DeepCopy()
	someKeys.ConfigMap.Items = []corev1.KeyToPath{{Key: "envoy.yaml", Path: "envoy.yaml"}}
	assert.Equal(t, `a deploymentOverlay mounts only some keys of the generated ConfigMap "gw" at /etc/envoy`,
		BootstrapMountProblem(deploymentMounting(someKeys, atEtcEnvoy), "gw"))

	// A container whose INPUT_CONF names a file outside /etc/envoy, or is not a literal
	// path, reads that file, not the mounted ConfigMap. A path inside /etc/envoy is in
	// the mounted ConfigMap.
	withInputConf := func(env corev1.EnvVar) []client.Object {
		objs := deploymentMounting(generated, atEtcEnvoy)
		objs[0].(*appsv1.Deployment).Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{env}
		return objs
	}
	outside := `the kgateway-proxy container's env sets INPUT_CONF to a path outside /etc/envoy, so the proxy reads its bootstrap from that file and not from the generated ConfigMap "gw"`
	assert.Equal(t, outside, BootstrapMountProblem(withInputConf(corev1.EnvVar{Name: inputConfigEnv, Value: "/custom/envoy.yaml"}), "gw"))
	assert.Equal(t, outside, BootstrapMountProblem(withInputConf(corev1.EnvVar{Name: inputConfigEnv, Value: "/etc/envoy/../custom/envoy.yaml"}), "gw"))
	assert.Equal(t, outside, BootstrapMountProblem(withInputConf(corev1.EnvVar{Name: inputConfigEnv, ValueFrom: &corev1.EnvVarSource{}}), "gw"))
	assert.Empty(t, BootstrapMountProblem(withInputConf(corev1.EnvVar{Name: inputConfigEnv, Value: "/etc/envoy/envoy.yaml"}), "gw"))

	// A second volume mounted inside /etc/envoy over the file the proxy reads holds that
	// file: envoy.yaml, or the file INPUT_CONF names.
	withSecondMount := func(mount corev1.VolumeMount, env ...corev1.EnvVar) []client.Object {
		objs := deploymentMounting(generated, atEtcEnvoy)
		container := &objs[0].(*appsv1.Deployment).Spec.Template.Spec.Containers[0]
		container.VolumeMounts = append(container.VolumeMounts, mount)
		container.Env = env
		return objs
	}
	customInputConf := corev1.EnvVar{Name: inputConfigEnv, Value: "/etc/envoy/custom/envoy.yaml"}
	assert.Equal(t, `the kgateway-proxy container mounts volume "custom" at /etc/envoy/envoy.yaml, so the proxy reads its bootstrap file /etc/envoy/envoy.yaml from that volume and not from the generated ConfigMap "gw"`,
		BootstrapMountProblem(withSecondMount(corev1.VolumeMount{Name: "custom", MountPath: "/etc/envoy/envoy.yaml", SubPath: "envoy.yaml"}), "gw"))
	assert.Equal(t, `the kgateway-proxy container mounts volume "custom" at /etc/envoy/custom, so the proxy reads its bootstrap file /etc/envoy/custom/envoy.yaml from that volume and not from the generated ConfigMap "gw"`,
		BootstrapMountProblem(withSecondMount(corev1.VolumeMount{Name: "custom", MountPath: "/etc/envoy/custom/"}, customInputConf), "gw"))

	// A second volume mounted elsewhere, inside or outside /etc/envoy, leaves the file
	// the proxy reads in the generated ConfigMap.
	assert.Empty(t, BootstrapMountProblem(withSecondMount(corev1.VolumeMount{Name: "certs", MountPath: "/etc/envoy/certs"}), "gw"))
	assert.Empty(t, BootstrapMountProblem(withSecondMount(corev1.VolumeMount{Name: "custom", MountPath: "/etc/envoy/custom"}), "gw"))
	assert.Empty(t, BootstrapMountProblem(withSecondMount(corev1.VolumeMount{Name: "certs", MountPath: "/etc/envoy/certs"}, customInputConf), "gw"))
	assert.Empty(t, BootstrapMountProblem(withSecondMount(corev1.VolumeMount{Name: "etc", MountPath: "/etc"}), "gw"))

	// Without the proxy container or a Deployment, nothing mounts it.
	assert.Equal(t, "a deploymentOverlay removes the kgateway-proxy container",
		BootstrapMountProblem([]client.Object{&appsv1.Deployment{}}, "gw"))
	assert.Equal(t, "no proxy Deployment is rendered", BootstrapMountProblem(nil, "gw"))
}

// proxyEnv returns the value of the environment variable name on the rendered proxy
// container, or "" when it is not set.
func proxyEnv(t *testing.T, objs []client.Object, name string) string {
	t.Helper()
	deps := objectsOf[*appsv1.Deployment](objs)
	require.Len(t, deps, 1)
	for _, c := range deps[0].Spec.Template.Spec.Containers {
		if c.Name != wellknown.KgatewayContainerName {
			continue
		}
		for _, e := range c.Env {
			if e.Name == name {
				return e.Value
			}
		}
	}
	return ""
}

func TestBootstrapOverlaysRevisionKey(t *testing.T) {
	// Render a Gateway without overlays.
	objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
	require.NoError(t, err)

	// The generated bootstrap is also under a key named by Helm's checksum, which
	// INPUT_CONF points the proxy at, so adding an overlay later changes only the next key.
	cm := findBootstrapConfigMap(objs)
	key := "envoy-" + checksum(t, objs) + ".yaml"
	assert.Equal(t, cm.Data[bootstrapConfigKey], cm.Data[key])
	assert.Equal(t, "/etc/envoy/"+key, proxyEnv(t, objs, inputConfigEnv))
	assert.Equal(t, []string{key}, revisionKeys(cm), "a Gateway without overlays should have one revision key")

	// Render a Gateway with overlays.
	objs, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)

	// The merged bootstrap is also under a key named by the pod template's checksum,
	// which INPUT_CONF points the proxy at.
	cm = findBootstrapConfigMap(objs)
	key = "envoy-" + checksum(t, objs) + ".yaml"
	assert.Equal(t, cm.Data[bootstrapConfigKey], cm.Data[key])
	assert.Equal(t, "/etc/envoy/"+key, proxyEnv(t, objs, inputConfigEnv))
}

// revisionKeys returns the revision keys of cm, sorted.
func revisionKeys(cm *corev1.ConfigMap) []string {
	var keys []string
	for key := range cm.Data {
		if strings.HasPrefix(key, "envoy-") {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// withoutRevisions removes the revision keys and INPUT_CONF that rendering wrote, for
// a test that writes revisions itself.
func withoutRevisions(objs []client.Object) []client.Object {
	cm := findBootstrapConfigMap(objs)
	for _, key := range revisionKeys(cm) {
		delete(cm.Data, key)
	}
	for _, dep := range objectsOf[*appsv1.Deployment](objs) {
		for i := range dep.Spec.Template.Spec.Containers {
			c := &dep.Spec.Template.Spec.Containers[i]
			c.Env = slices.DeleteFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == inputConfigEnv })
		}
	}
	return objs
}

func TestWriteBootstrapRevision(t *testing.T) {
	// render returns a Gateway's objects with overlays applied, and their outcome.
	render := func(t *testing.T, overlay string) ([]client.Object, *BootstrapOverlayOutcome) {
		t.Helper()
		objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
		require.NoError(t, err)
		objs = withoutRevisions(objs)
		outcome, err := ApplyBootstrapOverlays(t.Context(), objs, []bootstrapoverlay.Overlay{testOverlay(t, overlay)}, BootstrapOverlayConfig{})
		require.NoError(t, err)
		return objs, outcome
	}

	t.Run("keeps the revisions in use", func(t *testing.T) {
		// The live ConfigMap holds the revision a running pod uses, one an old
		// ReplicaSet uses, and one nothing uses.
		objs, outcome := render(t, `{stats_flush_interval: 10s}`)
		live := &corev1.ConfigMap{Data: map[string]string{
			bootstrapRevisionKey("pod-sum"):    "pod bootstrap",
			bootstrapRevisionKey("rs-sum"):     "replicaset bootstrap",
			bootstrapRevisionKey("unused-sum"): "unused bootstrap",
		}}
		err := WriteBootstrapRevision(objs, outcome, BootstrapRevisions{Live: live, InUse: []string{"rs-sum", "pod-sum", "pod-sum", "missing-sum"}})
		require.NoError(t, err)

		// The new revision and the two in use are written; the unused one and the one
		// the live ConfigMap does not hold are not.
		keys := revisionKeys(findBootstrapConfigMap(objs))
		assert.ElementsMatch(t, []string{bootstrapRevisionKey(outcome.checksum), bootstrapRevisionKey("pod-sum"), bootstrapRevisionKey("rs-sum")}, keys)
		assert.Equal(t, "pod bootstrap", findBootstrapConfigMap(objs).Data[bootstrapRevisionKey("pod-sum")])
	})

	t.Run("keeps the previous revision when there is room", func(t *testing.T) {
		// The live ConfigMap holds the revision it is current on, which nothing is known
		// to use, and one nothing uses.
		objs, outcome := render(t, `{stats_flush_interval: 10s}`)
		live := &corev1.ConfigMap{Data: map[string]string{
			bootstrapRevisionKey("previous-sum"): "previous bootstrap",
			bootstrapRevisionKey("unused-sum"):   "unused bootstrap",
		}}
		err := WriteBootstrapRevision(objs, outcome, BootstrapRevisions{Live: live, Previous: "previous-sum"})
		require.NoError(t, err)

		// The new revision and the previous one are written.
		cm := findBootstrapConfigMap(objs)
		assert.ElementsMatch(t, []string{bootstrapRevisionKey(outcome.checksum), bootstrapRevisionKey("previous-sum")}, revisionKeys(cm))
		assert.Equal(t, "previous bootstrap", cm.Data[bootstrapRevisionKey("previous-sum")])
	})

	t.Run("leaves out the previous revision when there is no room", func(t *testing.T) {
		// The live ConfigMap is current on a revision that leaves too little room for the
		// new one, and nothing is known to use it.
		objs, outcome := render(t, `{stats_flush_interval: 10s}`)
		live := &corev1.ConfigMap{Data: map[string]string{bootstrapRevisionKey("previous-sum"): strings.Repeat("x", maxConfigMapDataSize)}}
		err := WriteBootstrapRevision(objs, outcome, BootstrapRevisions{Live: live, Previous: "previous-sum"})

		// The new revision is written without it.
		require.NoError(t, err)
		assert.Equal(t, []string{bootstrapRevisionKey(outcome.checksum)}, revisionKeys(findBootstrapConfigMap(objs)))
	})

	t.Run("earlier revisions exceed the size limit", func(t *testing.T) {
		// The live ConfigMap holds a revision a running pod uses that leaves too little
		// room for the new one.
		objs, outcome := render(t, `{stats_flush_interval: 10s}`)
		live := &corev1.ConfigMap{Data: map[string]string{bootstrapRevisionKey("old-sum"): strings.Repeat("x", maxConfigMapDataSize)}}
		err := WriteBootstrapRevision(objs, outcome, BootstrapRevisions{Live: live, InUse: []string{"old-sum"}})

		// The error names the earlier revision's size and how to free it, and keeps the
		// managed paths known.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapConfigMapTooLarge, overlayErr.Reason)
		assert.Equal(t, "gw", overlayErr.ConfigMapName)
		assert.True(t, overlayErr.pathsKnown())
		assert.ErrorContains(t, overlayErr, fmt.Sprintf("and 1 earlier bootstrap revisions that proxy pods or ReplicaSets still use take %d bytes", maxConfigMapDataSize))
		assert.ErrorContains(t, overlayErr, "once the Gateway's old ReplicaSets are scaled down")
	})

	t.Run("one large bootstrap exceeds the size limit", func(t *testing.T) {
		// Overlays whose bootstrap alone, written twice, exceeds the limit.
		objs, outcome := render(t, fmt.Sprintf(`{node: {metadata: {big: %q}}}`, strings.Repeat("x", maxConfigMapDataSize/2)))
		err := WriteBootstrapRevision(objs, outcome, BootstrapRevisions{})

		// The error says to make the overlays smaller.
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, err, &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapConfigMapTooLarge, overlayErr.Reason)
		assert.ErrorContains(t, overlayErr, "make the bootstrap overlay smaller")
	})
}

func TestWriteGeneratedBootstrapRevision(t *testing.T) {
	// render returns a Gateway's objects without overlays, before revisions are written.
	render := func(t *testing.T) []client.Object {
		t.Helper()
		objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
		require.NoError(t, err)
		return withoutRevisions(objs)
	}

	t.Run("writes the generated bootstrap and keeps the revisions in use", func(t *testing.T) {
		// The live ConfigMap holds the revision of a pod that ran overlays and one nothing uses.
		objs := render(t)
		live := &corev1.ConfigMap{Data: map[string]string{
			bootstrapRevisionKey("overlay-sum"): "merged bootstrap",
			bootstrapRevisionKey("unused-sum"):  "unused bootstrap",
		}}
		require.NoError(t, WriteGeneratedBootstrapRevision(objs, BootstrapRevisions{Live: live, InUse: []string{"overlay-sum"}}))

		// The generated bootstrap is written under Helm's checksum, and the proxy reads
		// it; the revision of the pod with the overlay is kept, so removing the overlays does not take
		// its bootstrap away.
		cm := findBootstrapConfigMap(objs)
		key := bootstrapRevisionKey(checksum(t, objs))
		assert.Equal(t, cm.Data[bootstrapConfigKey], cm.Data[key])
		assert.Equal(t, "/etc/envoy/"+key, proxyEnv(t, objs, inputConfigEnv))
		assert.ElementsMatch(t, []string{key, bootstrapRevisionKey("overlay-sum")}, revisionKeys(cm))
		assert.Equal(t, "merged bootstrap", cm.Data[bootstrapRevisionKey("overlay-sum")])

		// It writes no validation data.
		assert.NotContains(t, cm.Data, bootstrapoverlay.ValidationKey)
		assert.NotContains(t, podTemplateAnnotations(t, objs), bootstrapoverlay.ValidationAnnotation)
	})

	t.Run("sets a missing checksum", func(t *testing.T) {
		// A pod template without the checksum annotation.
		objs := render(t)
		delete(objectsOf[*appsv1.Deployment](objs)[0].Spec.Template.Annotations, bootstrapChecksumAnnotation)
		require.NoError(t, WriteGeneratedBootstrapRevision(objs, BootstrapRevisions{}))

		// The checksum is the bootstrap's SHA-256, and names the revision.
		cm := findBootstrapConfigMap(objs)
		sum := sha256.Sum256([]byte(cm.Data[bootstrapConfigKey]))
		assert.Equal(t, hex.EncodeToString(sum[:]), checksum(t, objs))
		assert.Equal(t, []string{bootstrapRevisionKey(checksum(t, objs))}, revisionKeys(cm))
	})

	t.Run("earlier revisions exceed the size limit", func(t *testing.T) {
		// The live ConfigMap holds a revision a running pod uses that leaves too little
		// room for the new one.
		objs := render(t)
		live := &corev1.ConfigMap{Data: map[string]string{bootstrapRevisionKey("old-sum"): strings.Repeat("x", maxConfigMapDataSize)}}
		err := WriteGeneratedBootstrapRevision(objs, BootstrapRevisions{Live: live, InUse: []string{"old-sum"}})

		// Without overlays there is no condition to report on, so the error is not a
		// BootstrapOverlayError; its event names the earlier revision's size.
		var overlayErr *BootstrapOverlayError
		assert.False(t, errors.As(err, &overlayErr), "a Gateway without overlays should not get a BootstrapOverlayError")
		var tooLarge *BootstrapConfigMapTooLargeError
		require.ErrorAs(t, err, &tooLarge)
		assert.Equal(t, "gw", tooLarge.ConfigMapName)
		assert.ErrorContains(t, tooLarge, fmt.Sprintf("and 1 earlier bootstrap revisions that proxy pods or ReplicaSets still use take %d bytes", maxConfigMapDataSize))
		require.Len(t, tooLarge.Events(), 1)
		evt := tooLarge.Events()[0]
		assert.Equal(t, corev1.EventTypeWarning, evt.Type)
		assert.Equal(t, conditions.KgatewayReasonBootstrapConfigMapTooLarge, evt.Reason)
		assert.Equal(t, "Deployed none of the Gateway's objects: "+tooLarge.Error(), evt.Note)
	})
}

func TestCurrentBootstrapRevision(t *testing.T) {
	// A ConfigMap is current on the revision whose key holds the content of envoy.yaml.
	cm := &corev1.ConfigMap{Data: map[string]string{
		bootstrapConfigKey:                   "current bootstrap",
		bootstrapRevisionKey("earlier-sum"):  "earlier bootstrap",
		bootstrapRevisionKey("current-sum"):  "current bootstrap",
		bootstrapoverlay.ValidationKey:       "current bootstrap",
		"envoy-not-a-revision":               "current bootstrap",
		bootstrapRevisionKey("") + ".backup": "current bootstrap",
	}}
	assert.Equal(t, "current-sum", currentBootstrapRevision(cm))

	// A ConfigMap that holds no revision with that content is current on none. Keys that
	// are not revision keys do not count.
	delete(cm.Data, bootstrapRevisionKey("current-sum"))
	assert.Empty(t, currentBootstrapRevision(cm))

	// Neither is a ConfigMap without envoy.yaml, or no ConfigMap.
	assert.Empty(t, currentBootstrapRevision(&corev1.ConfigMap{Data: map[string]string{bootstrapRevisionKey("sum"): ""}}))
	assert.Empty(t, currentBootstrapRevision(nil))
}

func TestKeepBootstrapRevisions(t *testing.T) {
	// The live ConfigMap holds the revision a pod that mounts the generated ConfigMap
	// uses, and one nothing uses.
	live := &corev1.ConfigMap{Data: map[string]string{
		bootstrapRevisionKey("pod-sum"):    "pod bootstrap",
		bootstrapRevisionKey("unused-sum"): "unused bootstrap",
	}}
	revs := BootstrapRevisions{Live: live, InUse: []string{"pod-sum"}}

	t.Run("without overlays", func(t *testing.T) {
		// Keep the revisions of a Gateway without overlays.
		objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
		require.NoError(t, err)
		objs = withoutRevisions(objs)
		require.NoError(t, KeepBootstrapRevisions(objs, nil, revs))

		// Only the revision in use is written, and the proxy reads envoy.yaml from what it mounts.
		assert.Equal(t, []string{bootstrapRevisionKey("pod-sum")}, revisionKeys(findBootstrapConfigMap(objs)))
		assert.Empty(t, proxyEnv(t, objs, inputConfigEnv))
	})

	t.Run("with overlays", func(t *testing.T) {
		// Keep the revisions of a Gateway with overlays.
		objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
		require.NoError(t, err)
		objs = withoutRevisions(objs)
		outcome, err := ApplyBootstrapOverlays(t.Context(), objs, []bootstrapoverlay.Overlay{testOverlay(t, `{stats_flush_interval: 10s}`)}, BootstrapOverlayConfig{})
		require.NoError(t, err)
		require.NoError(t, KeepBootstrapRevisions(objs, outcome, revs))

		// Only the revision in use is written, with no validation data.
		cm := findBootstrapConfigMap(objs)
		assert.Equal(t, []string{bootstrapRevisionKey("pod-sum")}, revisionKeys(cm))
		assert.NotContains(t, cm.Data, bootstrapoverlay.ValidationKey)
		assert.Empty(t, proxyEnv(t, objs, inputConfigEnv))

		// A size failure is reported on the condition.
		big := BootstrapRevisions{Live: &corev1.ConfigMap{Data: map[string]string{bootstrapRevisionKey("pod-sum"): strings.Repeat("x", maxConfigMapDataSize)}}, InUse: []string{"pod-sum"}}
		var overlayErr *BootstrapOverlayError
		require.ErrorAs(t, KeepBootstrapRevisions(objs, outcome, big), &overlayErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapConfigMapTooLarge, overlayErr.Reason)
	})
}

func TestBootstrapRevisionsLiveConfigMap(t *testing.T) {
	gw := types.NamespacedName{Namespace: defaultNamespace, Name: "gw"}

	// liveConfigMap returns the Gateway's generated ConfigMap holding a revision for each of sums.
	liveConfigMap := func(sums ...string) *corev1.ConfigMap {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: defaultNamespace},
			Data:       map[string]string{},
		}
		for _, sum := range sums {
			cm.Data[bootstrapRevisionKey(sum)] = sum + " bootstrap"
		}
		return cm
	}

	// countReads makes gp read the ConfigMap from cm instead of the API server, and
	// returns how many times it did.
	countReads := func(gp *GatewayParameters, cm *corev1.ConfigMap) *int {
		reads := 0
		gp.kgwParameters.getConfigMap = func(context.Context, string, string) (*corev1.ConfigMap, error) {
			reads++
			return cm, nil
		}
		return &reads
	}

	// currentOn makes cm current on the revision sum: envoy.yaml holds that revision's content.
	currentOn := func(cm *corev1.ConfigMap, sum string) *corev1.ConfigMap {
		cm.Data[bootstrapConfigKey] = cm.Data[bootstrapRevisionKey(sum)]
		return cm
	}

	t.Run("uses the informer copy when it holds the current revision and every earlier revision in use", func(t *testing.T) {
		// The informer holds the ConfigMap with the pod's revision and the one this
		// reconcile writes, which it is current on.
		cli := fake.NewClient(t, currentOn(liveConfigMap("pod-sum", "current-sum"), "current-sum"))
		gp := NewGatewayParameters(cli, defaultInputs(t))
		cli.RunAndWait(t.Context().Done())
		reads := countReads(gp, liveConfigMap("pod-sum"))

		// The informer copy is returned without a read from the API server.
		revs, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"pod-sum"}, "current-sum")
		require.NoError(t, err)
		require.NotNil(t, revs.Live, "the informer copy should be returned")
		assert.Equal(t, "pod-sum bootstrap", revs.Live.Data[bootstrapRevisionKey("pod-sum")])
		assert.Equal(t, "current-sum", revs.Previous, "the ConfigMap is current on the revision this reconcile writes")
		assert.Zero(t, *reads, "the API server should not be read")
	})

	t.Run("reads the API server when the informer copy lacks the current revision", func(t *testing.T) {
		// The informer copy is from before the last write: it is current on the pod's
		// revision. The API server's copy is current on a later revision that no pod or
		// ReplicaSet is known to use.
		cli := fake.NewClient(t, currentOn(liveConfigMap("pod-sum"), "pod-sum"))
		gp := NewGatewayParameters(cli, defaultInputs(t))
		cli.RunAndWait(t.Context().Done())
		reads := countReads(gp, currentOn(liveConfigMap("pod-sum", "previous-sum"), "previous-sum"))

		// The API server's copy is returned, and names the revision to keep once more.
		revs, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"pod-sum"}, "current-sum")
		require.NoError(t, err)
		require.NotNil(t, revs.Live, "the ConfigMap the API server holds should be read")
		assert.Equal(t, "previous-sum bootstrap", revs.Live.Data[bootstrapRevisionKey("previous-sum")])
		assert.Equal(t, "previous-sum", revs.Previous)
		assert.Equal(t, 1, *reads, "the API server should be read once")
	})

	t.Run("reads the API server when the informer has no ConfigMap", func(t *testing.T) {
		// A pod already runs the revision this reconcile writes, and neither the informer
		// nor the API server has a ConfigMap.
		cli := fake.NewClient(t)
		gp := NewGatewayParameters(cli, defaultInputs(t))
		cli.RunAndWait(t.Context().Done())
		reads := countReads(gp, nil)

		// The API server is read, and there is nothing live and no previous revision.
		revs, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"current-sum"}, "current-sum")
		require.NoError(t, err)
		assert.Nil(t, revs.Live)
		assert.Empty(t, revs.Previous)
		assert.Equal(t, 1, *reads, "the API server should be read once")
	})

	t.Run("reads the API server when the informer copy lacks a revision in use", func(t *testing.T) {
		// The informer copy predates the write of the pod's revision; the API server has it.
		cli := fake.NewClient(t, liveConfigMap())
		gp := NewGatewayParameters(cli, defaultInputs(t))
		cli.RunAndWait(t.Context().Done())
		reads := countReads(gp, liveConfigMap("pod-sum"))

		// The API server's copy is returned.
		revs, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"pod-sum"}, "current-sum")
		require.NoError(t, err)
		require.NotNil(t, revs.Live, "the ConfigMap the API server holds should be read")
		assert.Equal(t, "pod-sum bootstrap", revs.Live.Data[bootstrapRevisionKey("pod-sum")])
		assert.Equal(t, 1, *reads, "the API server should be read once")
	})

	t.Run("reads the API server when no revision is written", func(t *testing.T) {
		// The proxy does not read the generated ConfigMap, so this reconcile writes no
		// revision, and the informer copy lacks the pods' revision.
		cli := fake.NewClient(t, liveConfigMap())
		gp := NewGatewayParameters(cli, defaultInputs(t))
		cli.RunAndWait(t.Context().Done())
		reads := countReads(gp, liveConfigMap("pod-sum"))

		// Every revision in use counts, so the API server is read.
		revs, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"pod-sum"}, "")
		require.NoError(t, err)
		require.NotNil(t, revs.Live)
		assert.Equal(t, 1, *reads, "the API server should be read once")
	})

	t.Run("a missing ConfigMap is no live ConfigMap", func(t *testing.T) {
		// Neither the informer nor the API server has a ConfigMap for the Gateway.
		gp := NewGatewayParameters(fake.NewClient(t), defaultInputs(t))

		// The read succeeds with nothing live.
		revs, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"pod-sum"}, "current-sum")
		require.NoError(t, err)
		assert.Nil(t, revs.Live)
	})

	t.Run("a failed read fails the reconcile", func(t *testing.T) {
		// The API server returns an error other than NotFound.
		gp := NewGatewayParameters(fake.NewClient(t), defaultInputs(t))
		readErr := errors.New("connection refused")
		gp.kgwParameters.getConfigMap = func(context.Context, string, string) (*corev1.ConfigMap, error) {
			return nil, readErr
		}

		// The error names the ConfigMap and keeps the cause, rather than reading as no
		// ConfigMap, which would drop every earlier revision.
		_, err := gp.bootstrapRevisions(t.Context(), gw, nil, "gw", []string{"pod-sum"}, "current-sum")
		require.ErrorIs(t, err, ErrBootstrapConfigMapUnreadable)
		require.ErrorIs(t, err, readErr)
		assert.ErrorContains(t, err, "default/gw")
	})
}

func TestBootstrapRevisionsReplicaSets(t *testing.T) {
	gw := types.NamespacedName{Namespace: defaultNamespace, Name: "gw"}

	// replicaSet returns a ReplicaSet of the Gateway owned by the Deployment owner, with
	// replicas and a pod template whose bootstrap checksum is sum.
	replicaSet := func(name, owner string, replicas *int32, sum string) *appsv1.ReplicaSet {
		return &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultNamespace,
				Labels:    map[string]string{wellknown.GatewayNameLabel: gw.Name},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: owner}}, wellknown.DeploymentGVK),
				},
			},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: replicas,
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{bootstrapChecksumAnnotation: sum},
				}},
			},
		}
	}

	// The proxy Deployment has a ReplicaSet with pods, one scaled to a replica that has
	// no pod yet, one with replicas unset, and one scaled down; another Deployment of the
	// Gateway has a ReplicaSet too.
	cli := fake.NewClient(t,
		replicaSet("gw-current", "gw", new(int32(2)), "current-sum"),
		replicaSet("gw-no-pods", "gw", new(int32(1)), "no-pods-sum"),
		replicaSet("gw-unset", "gw", nil, "unset-sum"),
		replicaSet("gw-scaled-down", "gw", new(int32(0)), "scaled-down-sum"),
		replicaSet("other-current", "other", new(int32(1)), "other-sum"),
	)
	gp := NewGatewayParameters(cli, defaultInputs(t))
	cli.RunAndWait(t.Context().Done())

	// The revisions in use are the pods' and those of the proxy Deployment's ReplicaSets
	// that can still create a pod.
	rendered := []client.Object{&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: defaultNamespace}}}
	revs, err := gp.bootstrapRevisions(t.Context(), gw, rendered, "gw", []string{"pod-sum"}, "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pod-sum", "current-sum", "no-pods-sum", "unset-sum"}, revs.InUse)
}

// servicePorts returns the ports of the one rendered Service.
func servicePorts(t *testing.T, objs []client.Object) []int32 {
	t.Helper()
	svcs := objectsOf[*corev1.Service](objs)
	require.Len(t, svcs, 1)
	var out []int32
	for _, p := range svcs[0].Spec.Ports {
		out = append(out, p.Port)
	}
	return out
}

func TestReserveServicePorts(t *testing.T) {
	// service returns a Service with a port, targeting the same port, for each of ports.
	service := func(ports ...int32) []client.Object {
		svc := &corev1.Service{}
		for _, p := range ports {
			svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Port: p, TargetPort: intstr.FromInt32(p)})
		}
		return []client.Object{svc}
	}

	for name, tc := range map[string]struct {
		listenerPorts []int32
		bootstrap     []uint32
		wantPorts     []int32
		wantRemoved   []int32
		wantErr       string
	}{
		"no port is reserved": {
			listenerPorts: []int32{8080, 8443},
			wantPorts:     []int32{8080, 8443},
		},
		"a port the merged bootstrap listens on is left off": {
			listenerPorts: []int32{8080, 8443},
			bootstrap:     []uint32{8443, 19000},
			wantPorts:     []int32{8080},
			wantRemoved:   []int32{8443},
		},
		"bootstrap ports that would empty the Service are an error": {
			listenerPorts: []int32{8443},
			bootstrap:     []uint32{8443},
			wantErr:       "the merged bootstrap listens on every Gateway listener port (8443)",
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Reserve the ports on a Service that exposes every listener port.
			objs := service(tc.listenerPorts...)
			outcome := &BootstrapOverlayOutcome{BootstrapPorts: tc.bootstrap}
			err := ReserveServicePorts(objs, outcome, tc.listenerPorts)

			// An error names the ports and is an InvalidOverlay.
			if tc.wantErr != "" {
				overlayErr, ok := errors.AsType[*BootstrapOverlayError](err)
				require.True(t, ok, "expected a *BootstrapOverlayError, got %v", err)
				assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, overlayErr.Reason)
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}

			// Otherwise check what is left on the Service, and what the outcome reports.
			require.NoError(t, err)
			assert.Equal(t, tc.wantPorts, servicePorts(t, objs))
			assert.Equal(t, tc.wantRemoved, outcome.RemovedServicePorts)
		})
	}
}

func TestBootstrapOverlaysManagedPathModes(t *testing.T) {
	// The overlays replace the xDS cluster's connect timeout and add a runtime layer.
	overlay := `{static_resources: {clusters: [{name: xds_cluster, connect_timeout: 9s}]}, layered_runtime: {layers: [{name: user_runtime, static_layer: {}}]}}`
	xdsTimeout := func(bs *envoybootstrapv3.Bootstrap) int64 {
		for _, c := range bs.GetStaticResources().GetClusters() {
			if c.GetName() == "xds_cluster" {
				return c.GetConnectTimeout().GetSeconds()
			}
		}
		return 0
	}

	t.Run("WARN", func(t *testing.T) {
		// Render under WARN.
		objs, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), overlay),
			managedPaths:  apisettings.BootstrapOverlayManagedPathsWarn,
		}.render(t)
		require.NoError(t, err)

		// The managed path applies, and a Warning event names it.
		_, bs := renderedBootstrap(t, objs)
		assert.Equal(t, int64(9), xdsTimeout(bs))
		assert.Equal(t, conditions.KgatewayReasonBootstrapManagedPathsOverridden, bootstrapCondition(t, report).Reason)
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapManagedPathsOverridden,
			Action: applyBootstrapOverlaysAction,
			Note:   "Applied a bootstrap overlay that sets static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params), which kgateway manages.",
		}}, report.Events)
	})

	t.Run("PRUNE", func(t *testing.T) {
		// Render under PRUNE.
		objs, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), overlay),
			managedPaths:  apisettings.BootstrapOverlayManagedPathsPrune,
		}.render(t)
		require.NoError(t, err)

		// The managed path is dropped and the rest applies.
		_, bs := renderedBootstrap(t, objs)
		assert.NotEqual(t, int64(9), xdsTimeout(bs), "the xDS cluster should keep its generated timeout")
		assert.Equal(t, "user_runtime", bs.GetLayeredRuntime().GetLayers()[0].GetName())

		// The condition stays True and names the dropped path, as does a Warning event.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonBootstrapPathsIgnored, cond.Reason)
		assert.Contains(t, cond.Message, "These paths were dropped from it because kgateway manages them: static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params).")
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapPathsIgnored,
			Action: applyBootstrapOverlaysAction,
			Note:   "Dropped static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params) from the bootstrap overlay because kgateway manages it.",
		}}, report.Events)
	})

	t.Run("REJECT", func(t *testing.T) {
		// Render under REJECT.
		objs, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), overlay),
			managedPaths:  apisettings.BootstrapOverlayManagedPathsReject,
		}.render(t)

		// Nothing is rendered, and the error lists the denied path.
		overlayErr, ok := errors.AsType[*BootstrapOverlayError](err)
		require.True(t, ok, "expected a *BootstrapOverlayError, got %v", err)
		assert.Nil(t, objs)
		assert.Nil(t, report)
		assert.Equal(t, conditions.KgatewayReasonBootstrapPathsDenied, overlayErr.Reason)
		assert.Equal(t, []bootstrapoverlay.ManagedPath{{
			Path:   "static_resources.clusters[name=xds_cluster]",
			Source: gatewayParamsSource(),
		}}, overlayErr.ManagedPaths)

		// A Warning event names each denied path.
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapPathsDenied,
			Action: applyBootstrapOverlaysAction,
			Note:   "Applied no bootstrap overlay: an overlay sets static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params), which kgateway manages, and KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS is REJECT.",
		}}, overlayErr.Events())
	})

	t.Run("REJECT without managed paths", func(t *testing.T) {
		// Overlays that set no managed path apply under REJECT.
		_, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`),
			managedPaths:  apisettings.BootstrapOverlayManagedPathsReject,
		}.render(t)
		require.NoError(t, err)
		assert.Equal(t, conditions.KgatewayReasonBootstrapOverlayApplied, bootstrapCondition(t, report).Reason)
	})
}

func TestBootstrapOverlayEvents(t *testing.T) {
	t.Run("not mounted", func(t *testing.T) {
		// Render a Gateway whose deploymentOverlay mounts another ConfigMap, with overlays
		// that also set a managed path.
		gwp := withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster, connect_timeout: 9s}]}}`)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`)}}
		_, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
		require.NoError(t, err)

		// The only event is the condition's: the overlays have no other effect.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapNotMounted,
			Action: applyBootstrapOverlaysAction,
			Note:   cond.Message,
		}}, report.Events)
	})

	t.Run("invalid overlay", func(t *testing.T) {
		// Render a Gateway whose overlay does not parse.
		_, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)}.render(t)
		overlayErr, ok := errors.AsType[*BootstrapOverlayError](err)
		require.True(t, ok, "expected a *BootstrapOverlayError, got %v", err)

		// One Warning event carries the error.
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapInvalidOverlay,
			Action: applyBootstrapOverlaysAction,
			Note:   "Applied no bootstrap overlay: " + overlayErr.Error(),
		}}, overlayErr.Events())
	})
}

func TestBootstrapOverlayManagedPathsMetric(t *testing.T) {
	// Collect metrics for this test only.
	wasActive := metrics.Active()
	metrics.SetActive(true)
	ResetMetrics()
	t.Cleanup(func() {
		ResetMetrics()
		metrics.SetActive(wasActive)
	})
	gw := types.NamespacedName{Namespace: defaultNamespace, Name: "gw"}
	assertSeries := func(t *testing.T, want string) {
		t.Helper()
		expected := "# HELP kgateway_controller_bootstrap_overlay_managed_paths Control-plane-managed bootstrap paths that a Gateway's GatewayParameters bootstrap overlay sets. The value is always 1.\n" +
			"# TYPE kgateway_controller_bootstrap_overlay_managed_paths gauge\n" + want
		if want == "" {
			expected = ""
		}
		require.NoError(t, metricstest.CollectAndCompare(bootstrapOverlayManagedPaths, strings.NewReader(expected),
			"kgateway_controller_bootstrap_overlay_managed_paths"))
	}

	// Overlays that set two managed paths give one series per path.
	_, _, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster}]}, admin: {address: {socket_address: {address: 0.0.0.0, port_value: 19000}}}}`),
	}.render(t)
	require.NoError(t, err)
	assertSeries(t, `kgateway_controller_bootstrap_overlay_managed_paths{name="gw",namespace="default",path="admin.address"} 1
kgateway_controller_bootstrap_overlay_managed_paths{name="gw",namespace="default",path="static_resources.clusters[name=xds_cluster]"} 1
`)

	// A REJECT error still reports what the overlays set, replacing the earlier series.
	_, _, err = bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster}]}}`),
		managedPaths:  apisettings.BootstrapOverlayManagedPathsReject,
	}.render(t)
	require.Error(t, err)
	assertSeries(t, `kgateway_controller_bootstrap_overlay_managed_paths{name="gw",namespace="default",path="static_resources.clusters[name=xds_cluster]"} 1
`)

	// An invalid overlay leaves the series as they were, since its paths are unknown.
	_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)}.render(t)
	require.Error(t, err)
	assertSeries(t, `kgateway_controller_bootstrap_overlay_managed_paths{name="gw",namespace="default",path="static_resources.clusters[name=xds_cluster]"} 1
`)

	// Overlays that set no managed path remove the Gateway's series.
	_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	assertSeries(t, "")

	// Removing the overlays, or deleting the Gateway, removes them too.
	_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster}]}}`)}.render(t)
	require.NoError(t, err)
	_, _, err = bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
	require.NoError(t, err)
	assertSeries(t, "")
	recordBootstrapOverlayManagedPaths(gw, []bootstrapoverlay.ManagedPath{{Path: "node"}})
	ForgetBootstrapOverlayMetrics(gw)
	assertSeries(t, "")
}

func gatewayParamsSource() bootstrapoverlay.Source {
	return bootstrapoverlay.Source{Namespace: defaultNamespace, Name: gatewayParamsName}
}

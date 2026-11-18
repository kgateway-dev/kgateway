package deployer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
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
	// validator validates the patched bootstrap; nil for none.
	validator validator.BootstrapValidator
	// managedPaths is the KGW_BOOTSTRAP_PATCH_MANAGED_PATHS mode; empty means WARN.
	managedPaths apisettings.BootstrapPatchManagedPathsMode
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
	inputs.BootstrapPatchManagedPaths = tc.managedPaths
	gwp := NewGatewayParameters(fakeClient, inputs)
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
		Patches: &kgateway.BootstrapPatches{Overlay: &kgateway.BootstrapOverlay{Value: apiextensionsv1.JSON{Raw: raw}}},
	}}
	return gwp
}

// testOverlay returns the given YAML as an overlay from the Gateway's GatewayParameters.
func testOverlay(t *testing.T, overlayYAML string) bootstrappatch.Overlay {
	t.Helper()
	raw, err := yaml.YAMLToJSON([]byte(overlayYAML))
	require.NoError(t, err)
	return bootstrappatch.Overlay{Source: types.NamespacedName{Namespace: defaultNamespace, Name: gatewayParamsName}, Raw: raw}
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
	bs, err := bootstrappatch.ParseBootstrap([]byte(cm.Data[bootstrapConfigKey]))
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
	assert.Equal(t, []string{conditions.KgatewayConditionBootstrapPatchApplied}, report.OwnedConditionTypes)
	require.Len(t, report.Conditions, 1)
	return report.Conditions[0]
}

func TestBootstrapPatchesNotSet(t *testing.T) {
	// Render a Gateway whose GatewayParameters have no bootstrap patches.
	objs, report, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
	require.NoError(t, err)

	// The bootstrap is the Helm-rendered text, byte for byte, with Helm's checksum.
	envoyYaml, _ := renderedBootstrap(t, objs)
	sum := sha256.Sum256([]byte(envoyYaml))
	assert.Equal(t, hex.EncodeToString(sum[:]), checksum(t, objs), "checksum should be Helm's SHA-256 of the rendered bootstrap")
	assert.Contains(t, envoyYaml, "port_value: 19000\n", "the bootstrap should keep the Helm template's formatting")

	// Without patches the translator validates against Envoy's defaults, so there is no
	// validation data to write.
	assert.NotContains(t, findBootstrapConfigMap(objs).Data, bootstrappatch.ValidationKey)
	assert.NotContains(t, podTemplateAnnotations(t, objs), bootstrappatch.ValidationAnnotation)

	// The report clears the condition, sets none, and records no events.
	require.NotNil(t, report)
	assert.Empty(t, report.Conditions)
	assert.Equal(t, []string{conditions.KgatewayConditionBootstrapPatchApplied}, report.OwnedConditionTypes)
	assert.Empty(t, report.Events)
}

func TestBootstrapPatchesApplied(t *testing.T) {
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

	// The checksum matches the patched bootstrap.
	sum := sha256.Sum256([]byte(envoyYaml))
	assert.Equal(t, hex.EncodeToString(sum[:]), checksum(t, objs))

	// The condition reports the patches as applied.
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapPatchApplied, cond.Reason)
	assert.Equal(t, int64(3), cond.ObservedGeneration)
	assert.Equal(t, "The bootstrap patches are applied.", cond.Message)
	assert.Empty(t, report.Events, "patches with nothing to warn about should record no events")

	// Rendering the same patches again gives the same checksum; changing them changes it.
	again, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)
	require.NoError(t, err)
	assert.Equal(t, checksum(t, objs), checksum(t, again), "identical patches should give an identical checksum")
	changed, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	assert.NotEqual(t, checksum(t, objs), checksum(t, changed), "different patches should change the checksum")
}

func TestBootstrapPatchesValidationData(t *testing.T) {
	// Render a Gateway whose patches add a static runtime layer and a static listener,
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

	// The ConfigMap holds the static layer, and exactly the ports the patched bootstrap
	// listens on: 8082 is no longer one of them.
	data, err := bootstrappatch.DecodeValidationData(findBootstrapConfigMap(objs).Data[bootstrappatch.ValidationKey])
	require.NoError(t, err)
	assert.JSONEq(t, `{"layered_runtime":{"layers":[{"name":"limits","static_layer":{"re2.max_program_size.error_level":1000}}]}}`, string(data.Bootstrap))
	assert.Equal(t, []uint32{8083, 9000, 9091, 19000}, data.ReservedPorts)

	// The pod template carries the same subset, byte for byte.
	assert.Equal(t, string(data.Bootstrap), podTemplateAnnotations(t, objs)[bootstrappatch.ValidationAnnotation])

	// Patches without a static layer write the empty subset, which the translator
	// treats as Envoy's defaults.
	objs, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	assert.Equal(t, bootstrappatch.EmptyValidationSubset, podTemplateAnnotations(t, objs)[bootstrappatch.ValidationAnnotation])
}

func TestBootstrapPatchesValidationSubsetTooLarge(t *testing.T) {
	// Render a Gateway whose patches set a static runtime layer larger than the
	// annotation limit.
	overlay := fmt.Sprintf(`{layered_runtime: {layers: [{name: big, static_layer: {key: %q}}]}}`,
		strings.Repeat("x", bootstrappatch.MaxValidationSubsetSize))
	objs, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)

	// Nothing is rendered, and the error names the limit.
	var patchErr *BootstrapPatchError
	require.ErrorAs(t, err, &patchErr)
	assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, patchErr.Reason)
	assert.ErrorContains(t, patchErr, "the static runtime layers of the patched bootstrap take")
	assert.Nil(t, objs)
}

func TestBootstrapPatchesGatewayClassThenGateway(t *testing.T) {
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

func TestBootstrapPatchesManagedPath(t *testing.T) {
	// Render a Gateway whose patches replace the xDS cluster.
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

func TestBootstrapPatchesInvalid(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Render a Gateway with invalid patches.
			objs, report, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), tt.overlay)}.render(t)

			// Nothing is rendered, and the error carries the condition reason.
			var patchErr *BootstrapPatchError
			require.ErrorAs(t, err, &patchErr)
			assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, patchErr.Reason)
			assert.Equal(t, "gw", patchErr.ConfigMapName)
			assert.ErrorContains(t, patchErr, tt.wantErr)
			assert.Nil(t, objs)
			assert.Nil(t, report)
		})
	}
}

// fakeBootstrapValidator records the bootstrap it validates and returns a programmed
// response.
type fakeBootstrapValidator struct {
	got    *envoybootstrapv3.Bootstrap
	result validator.BootstrapResult
	err    error
}

func (f *fakeBootstrapValidator) ValidateBootstrap(_ context.Context, bs *envoybootstrapv3.Bootstrap) (validator.BootstrapResult, error) {
	f.got = bs
	return f.result, f.err
}

func TestBootstrapPatchesValidation(t *testing.T) {
	gwp := func() *kgateway.GatewayParameters {
		return withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)
	}

	t.Run("accepted, with removed runtime guards", func(t *testing.T) {
		// Render a Gateway whose patched bootstrap Envoy accepts with a removed guard.
		v := &fakeBootstrapValidator{result: validator.BootstrapResult{RemovedRuntimeGuards: []string{"envoy.reloadable_features.a"}}}
		objs, report, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)
		require.NoError(t, err)

		// Envoy validated the merged bootstrap, which is what the ConfigMap holds.
		require.NotNil(t, v.got)
		assert.Equal(t, int64(10), v.got.GetStatsFlushInterval().GetSeconds())
		_, written := renderedBootstrap(t, objs)
		assert.True(t, proto.Equal(written, v.got), "the validated bootstrap should be the one written")

		// The patches apply, and the condition names the removed guard.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, conditions.KgatewayReasonBootstrapPatchApplied, cond.Reason)
		assert.Contains(t, cond.Message, "Envoy no longer has the runtime guard(s) envoy.reloadable_features.a")
	})

	t.Run("rejected", func(t *testing.T) {
		// Render a Gateway whose patched bootstrap Envoy rejects.
		v := &fakeBootstrapValidator{err: fmt.Errorf("%w: error initializing configuration '': bad", validator.ErrInvalidBootstrap)}
		objs, report, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)

		// Nothing is rendered, and the error carries the reason and Envoy's message.
		var patchErr *BootstrapPatchError
		require.ErrorAs(t, err, &patchErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapValidationFailed, patchErr.Reason)
		assert.Equal(t, "gw", patchErr.ConfigMapName)
		assert.EqualError(t, patchErr, "Envoy rejected the bootstrap: error initializing configuration '': bad")
		assert.Nil(t, objs)
		assert.Nil(t, report)
	})

	t.Run("Envoy did not finish", func(t *testing.T) {
		// Render a Gateway when Envoy is killed while validating.
		v := &fakeBootstrapValidator{err: errors.New("Envoy was killed while validating the bootstrap")}
		_, _, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)

		// The patches are reported as not validated, and the reconcile retries.
		var patchErr *BootstrapPatchError
		require.ErrorAs(t, err, &patchErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapValidationUnavailable, patchErr.Reason)
		assert.True(t, patchErr.Retry, "validating again can succeed")
		assert.EqualError(t, patchErr, "could not validate the patched bootstrap: Envoy was killed while validating the bootstrap; the controller retries")
	})

	t.Run("the controller cannot run Envoy", func(t *testing.T) {
		// Render a Gateway when the Envoy binary is missing.
		v := &fakeBootstrapValidator{err: fmt.Errorf("%w: exec: no such file", validator.ErrBootstrapValidatorUnavailable)}
		_, _, err := bootstrapTestCase{gatewayParams: gwp(), validator: v}.render(t)

		// The patches are reported as not validated, without a retry: the restart that
		// a change to the controller's Deployment causes reconciles the Gateway again.
		var patchErr *BootstrapPatchError
		require.ErrorAs(t, err, &patchErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapValidationUnavailable, patchErr.Reason)
		assert.False(t, patchErr.Retry)
		assert.EqualError(t, patchErr, "the controller cannot run Envoy to validate the bootstrap: exec: no such file; the controller retries after its Deployment or settings change")
	})

	t.Run("the controller is stopping", func(t *testing.T) {
		// Render a Gateway's objects without patches, then apply patches with a context
		// that has ended.
		objs, _, err := bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		v := &fakeBootstrapValidator{err: context.Canceled}
		_, err = ApplyBootstrapPatches(ctx, objs, []bootstrappatch.Overlay{testOverlay(t, `{stats_flush_interval: 10s}`)}, BootstrapPatchConfig{Validator: v})

		// The error is the context's, not a report on the patches.
		require.ErrorIs(t, err, context.Canceled)
		var patchErr *BootstrapPatchError
		assert.False(t, errors.As(err, &patchErr))
	})

	t.Run("a controller failure", func(t *testing.T) {
		// Apply patches to rendered objects that have no bootstrap ConfigMap.
		_, err := ApplyBootstrapPatches(t.Context(), nil, []bootstrappatch.Overlay{testOverlay(t, `{stats_flush_interval: 10s}`)}, BootstrapPatchConfig{})

		// The failure is reported as the controller's, without a retry or a ConfigMap name.
		var patchErr *BootstrapPatchError
		require.ErrorAs(t, err, &patchErr)
		assert.Equal(t, conditions.KgatewayReasonBootstrapControllerError, patchErr.Reason)
		assert.False(t, patchErr.Retry)
		assert.Empty(t, patchErr.ConfigMapName)
		assert.EqualError(t, patchErr, "the controller failed to apply the bootstrap patches; this is a kgateway defect, not a problem with the patches: no bootstrap ConfigMap was rendered")
	})

	t.Run("not run without patches", func(t *testing.T) {
		v := &fakeBootstrapValidator{}
		_, _, err := bootstrapTestCase{gatewayParams: gatewayParams(), validator: v}.render(t)
		require.NoError(t, err)
		assert.Nil(t, v.got)
	})
}

func TestBootstrapPatchesListenerPortCollision(t *testing.T) {
	// Render a Gateway with listeners on 8080 and 8443, whose patches add a static
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

	// When every listener port collides, nothing is deployed, as for any Gateway with no valid ports.
	_, _, err = bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), overlay),
		listenerPorts: []gwv1.PortNumber{8443},
	}.render(t)
	assert.ErrorIs(t, err, ErrNoValidPorts)
}

func TestBootstrapPatchesDefaultListenerPorts(t *testing.T) {
	// Render a Gateway with listeners on 8080 and 8082, whose patches leave the
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

	// 8082 is left off the Service, as without patches, and the condition says so.
	assert.Equal(t, []int32{8080}, serviceOf(objs))
	assert.Contains(t, bootstrapCondition(t, report).Message, "uses Gateway listener port(s) 8082")

	// Patches that move the readiness listener to 8083 free 8082 for the Gateway
	// listener, so the Service exposes it.
	moved := `
static_resources:
  listeners:
  - name: readiness_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 8083}}`
	objs, report, err = bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), moved),
		listenerPorts: []gwv1.PortNumber{8080, 8082},
	}.render(t)
	require.NoError(t, err)
	assert.Equal(t, []int32{8080, 8082}, serviceOf(objs))
	assert.NotContains(t, bootstrapCondition(t, report).Message, "Gateway listener port")
}

func TestBootstrapPatchesNotMounted(t *testing.T) {
	// Render a Gateway whose patches apply but whose deploymentOverlay mounts another
	// ConfigMap in the bootstrap volume.
	gwp := withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)
	gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
		`{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`)}}
	objs, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
	require.NoError(t, err)

	// The objects deploy as the overlay asks, and the condition says the patches are not in effect.
	assert.Equal(t, "my-bootstrap", BootstrapMountedConfigMap(objs))
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapNotMounted, cond.Reason)
	assert.Contains(t, cond.Message, `mounts ConfigMap "my-bootstrap"`)
}

func TestBootstrapPatchesManagedPathModes(t *testing.T) {
	// The patches replace the xDS cluster's connect timeout and add a runtime layer.
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
			managedPaths:  apisettings.BootstrapPatchManagedPathsWarn,
		}.render(t)
		require.NoError(t, err)

		// The managed path applies, and a Warning event names it.
		_, bs := renderedBootstrap(t, objs)
		assert.Equal(t, int64(9), xdsTimeout(bs))
		assert.Equal(t, conditions.KgatewayReasonBootstrapManagedPathsOverridden, bootstrapCondition(t, report).Reason)
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapManagedPathsOverridden,
			Action: applyBootstrapPatchesAction,
			Note:   "Applied bootstrap patches that set static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params), which the control plane manages.",
		}}, report.Events)
	})

	t.Run("PRUNE", func(t *testing.T) {
		// Render under PRUNE.
		objs, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), overlay),
			managedPaths:  apisettings.BootstrapPatchManagedPathsPrune,
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
		assert.Contains(t, cond.Message, "These paths were dropped from them because the control plane manages them: static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params).")
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapPathsIgnored,
			Action: applyBootstrapPatchesAction,
			Note:   "Dropped static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params) from the bootstrap patches because the control plane manages it.",
		}}, report.Events)
	})

	t.Run("REJECT", func(t *testing.T) {
		// Render under REJECT.
		objs, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), overlay),
			managedPaths:  apisettings.BootstrapPatchManagedPathsReject,
		}.render(t)

		// Nothing is rendered, and the error lists the denied path.
		patchErr, ok := errors.AsType[*BootstrapPatchError](err)
		require.True(t, ok, "expected a *BootstrapPatchError, got %v", err)
		assert.Nil(t, objs)
		assert.Nil(t, report)
		assert.Equal(t, conditions.KgatewayReasonBootstrapPathsDenied, patchErr.Reason)
		assert.Equal(t, []bootstrappatch.ManagedPath{{
			Path:   "static_resources.clusters[name=xds_cluster]",
			Source: gatewayParamsKey(),
		}}, patchErr.ManagedPaths)

		// A Warning event names each denied path.
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapPathsDenied,
			Action: applyBootstrapPatchesAction,
			Note:   "Applied none of the bootstrap patches: they set static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params), which the control plane manages, and the controller is configured to reject such patches.",
		}}, patchErr.Events())
	})

	t.Run("REJECT without managed paths", func(t *testing.T) {
		// Patches that set no managed path apply under REJECT.
		_, report, err := bootstrapTestCase{
			gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`),
			managedPaths:  apisettings.BootstrapPatchManagedPathsReject,
		}.render(t)
		require.NoError(t, err)
		assert.Equal(t, conditions.KgatewayReasonBootstrapPatchApplied, bootstrapCondition(t, report).Reason)
	})
}

func TestBootstrapPatchesCuratedOverride(t *testing.T) {
	// Render a Gateway whose patches replace the DNS resolver that
	// envoyContainer.bootstrap.dnsResolver generates.
	overlay := `
typed_dns_resolver_config:
  name: envoy.network.dns_resolver.cares
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig
    use_resolvers_as_fallback: true`
	objs, report, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)
	require.NoError(t, err)

	// The overlay's resolver replaces the generated one.
	_, bs := renderedBootstrap(t, objs)
	require.NotNil(t, bs.GetTypedDnsResolverConfig())

	// The condition keeps its reason and names the field and the path.
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapPatchApplied, cond.Reason)
	assert.Contains(t, cond.Message,
		"typed_dns_resolver_config replaces what envoyContainer.bootstrap.dnsResolver generates (GatewayParameters default/gw-params)")

	// A Warning event names the replaced field.
	assert.Equal(t, []deployer.ReportEvent{{
		Type:   corev1.EventTypeWarning,
		Reason: curatedFieldOverriddenReason,
		Action: applyBootstrapPatchesAction,
		Note:   "The bootstrap patches replace generated configuration: typed_dns_resolver_config replaces what envoyContainer.bootstrap.dnsResolver generates (GatewayParameters default/gw-params).",
	}}, report.Events)
}

func TestBootstrapPatchesUnvalidatedRuntimeLayers(t *testing.T) {
	// Render a Gateway whose patches add a disk layer and an RTDS layer that reads from
	// a cluster of their own, validated by Envoy.
	overlay := `
static_resources:
  clusters:
  - name: runtime_server
    connect_timeout: 1s
    type: STATIC
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config: {http2_protocol_options: {}}
    load_assignment:
      cluster_name: runtime_server
      endpoints: [{lb_endpoints: [{endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 18000}}}}]}]
layered_runtime:
  layers:
  - {name: from_disk, disk_layer: {symlink_root: /etc/runtime, subdirectory: envoy}}
  - name: from_rtds
    rtds_layer:
      name: from_rtds
      rtds_config:
        resource_api_version: V3
        api_config_source:
          api_type: GRPC
          transport_api_version: V3
          grpc_services: [{envoy_grpc: {cluster_name: runtime_server}}]`
	_, report, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), overlay),
		validator:     validator.NewDocker().(validator.BootstrapValidator),
	}.render(t)
	require.NoError(t, err)

	// The layers apply, and the condition keeps its reason and names both.
	cond := bootstrapCondition(t, report)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, conditions.KgatewayReasonBootstrapPatchApplied, cond.Reason)
	assert.Equal(t, "The bootstrap patches are applied. "+
		"The proxy loads the values of these runtime layers while it runs, so route validation uses Envoy's defaults for the keys they set: "+
		`disk layer "from_disk" (GatewayParameters default/gw-params), RTDS layer "from_rtds" (GatewayParameters default/gw-params).`,
		cond.Message)

	// A Warning event names each layer.
	assert.Equal(t, []deployer.ReportEvent{
		{
			Type:   corev1.EventTypeWarning,
			Reason: unvalidatedRuntimeLayerReason,
			Action: applyBootstrapPatchesAction,
			Note:   `The bootstrap patches add disk layer "from_disk" (GatewayParameters default/gw-params). The proxy loads its values while it runs, so route validation uses Envoy's defaults for the keys it sets.`,
		},
		{
			Type:   corev1.EventTypeWarning,
			Reason: unvalidatedRuntimeLayerReason,
			Action: applyBootstrapPatchesAction,
			Note:   `The bootstrap patches add RTDS layer "from_rtds" (GatewayParameters default/gw-params). The proxy loads its values while it runs, so route validation uses Envoy's defaults for the keys it sets.`,
		},
	}, report.Events)
}

func TestBootstrapPatchesRTDSLayerFromControlPlane(t *testing.T) {
	// Render a Gateway whose patches add an RTDS layer that reads through ADS.
	_, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(),
		`{layered_runtime: {layers: [{name: rt, rtds_layer: {name: rt, rtds_config: {ads: {}}}}]}}`)}.render(t)

	// None of the patches apply, with reason InvalidOverlay.
	patchErr, ok := errors.AsType[*BootstrapPatchError](err)
	require.True(t, ok, "expected a *BootstrapPatchError, got %v", err)
	assert.Equal(t, conditions.KgatewayReasonBootstrapInvalidOverlay, patchErr.Reason)
	assert.ErrorContains(t, patchErr, `RTDS runtime layer "rt" requests its values from the kgateway control plane through ADS, which serves no runtime values`)
}

func TestBootstrapPatchEvents(t *testing.T) {
	t.Run("removed runtime guards", func(t *testing.T) {
		// Render a Gateway whose patched bootstrap Envoy accepts with removed guards.
		v := &fakeBootstrapValidator{result: validator.BootstrapResult{RemovedRuntimeGuards: []string{"envoy.reloadable_features.a", "envoy.reloadable_features.b"}}}
		_, report, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`), validator: v}.render(t)
		require.NoError(t, err)

		// One Warning event names them all.
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: removedRuntimeGuardsReason,
			Action: applyBootstrapPatchesAction,
			Note:   "Envoy no longer has the runtime guard(s) envoy.reloadable_features.a, envoy.reloadable_features.b, so their values have no effect; remove them from the bootstrap patches.",
		}}, report.Events)
	})

	t.Run("not mounted", func(t *testing.T) {
		// Render a Gateway whose deploymentOverlay mounts another ConfigMap, with patches
		// that also set a managed path.
		gwp := withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster, connect_timeout: 9s}]}}`)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template": {"spec": {"volumes": [{"name": "envoy-config", "configMap": {"name": "my-bootstrap"}}]}}}`)}}
		_, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
		require.NoError(t, err)

		// The only event is the condition's: the patches have no other effect.
		cond := bootstrapCondition(t, report)
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapNotMounted,
			Action: applyBootstrapPatchesAction,
			Note:   cond.Message,
		}}, report.Events)
	})

	t.Run("invalid overlay", func(t *testing.T) {
		// Render a Gateway whose overlay does not parse.
		_, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)}.render(t)
		patchErr, ok := errors.AsType[*BootstrapPatchError](err)
		require.True(t, ok, "expected a *BootstrapPatchError, got %v", err)

		// One Warning event carries the error.
		assert.Equal(t, []deployer.ReportEvent{{
			Type:   corev1.EventTypeWarning,
			Reason: conditions.KgatewayReasonBootstrapInvalidOverlay,
			Action: applyBootstrapPatchesAction,
			Note:   "Applied none of the bootstrap patches: " + patchErr.Error(),
		}}, patchErr.Events())
	})
}

func TestBootstrapPatchManagedPathsMetric(t *testing.T) {
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
		expected := "# HELP kgateway_controller_bootstrap_patch_managed_paths Control-plane-managed bootstrap paths that a Gateway's GatewayParameters bootstrap patches set. The value is always 1.\n" +
			"# TYPE kgateway_controller_bootstrap_patch_managed_paths gauge\n" + want
		if want == "" {
			expected = ""
		}
		require.NoError(t, metricstest.CollectAndCompare(bootstrapPatchManagedPaths, strings.NewReader(expected),
			"kgateway_controller_bootstrap_patch_managed_paths"))
	}

	// Patches that set two managed paths give one series per path.
	_, _, err := bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster}]}, admin: {address: {socket_address: {address: 0.0.0.0, port_value: 19000}}}}`),
	}.render(t)
	require.NoError(t, err)
	assertSeries(t, `kgateway_controller_bootstrap_patch_managed_paths{name="gw",namespace="default",path="admin.address"} 1
kgateway_controller_bootstrap_patch_managed_paths{name="gw",namespace="default",path="static_resources.clusters[name=xds_cluster]"} 1
`)

	// A REJECT error still reports what the patches set, replacing the earlier series.
	_, _, err = bootstrapTestCase{
		gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster}]}}`),
		managedPaths:  apisettings.BootstrapPatchManagedPathsReject,
	}.render(t)
	require.Error(t, err)
	assertSeries(t, `kgateway_controller_bootstrap_patch_managed_paths{name="gw",namespace="default",path="static_resources.clusters[name=xds_cluster]"} 1
`)

	// An invalid overlay leaves the series as they were, since its paths are unknown.
	_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)}.render(t)
	require.Error(t, err)
	assertSeries(t, `kgateway_controller_bootstrap_patch_managed_paths{name="gw",namespace="default",path="static_resources.clusters[name=xds_cluster]"} 1
`)

	// Patches that set no managed path remove the Gateway's series.
	_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	assertSeries(t, "")

	// Removing the patches, or deleting the Gateway, removes them too.
	_, _, err = bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{static_resources: {clusters: [{name: xds_cluster}]}}`)}.render(t)
	require.NoError(t, err)
	_, _, err = bootstrapTestCase{gatewayParams: gatewayParams()}.render(t)
	require.NoError(t, err)
	assertSeries(t, "")
	recordBootstrapPatchManagedPaths(gw, []bootstrappatch.ManagedPath{{Path: "node"}})
	ForgetBootstrapPatchMetrics(gw)
	assertSeries(t, "")
}

func gatewayParamsKey() types.NamespacedName {
	return types.NamespacedName{Namespace: defaultNamespace, Name: gatewayParamsName}
}

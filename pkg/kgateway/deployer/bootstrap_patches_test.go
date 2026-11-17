package deployer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
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
	gwp := NewGatewayParameters(fakeClient, defaultInputs(t, gwc, gw))
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
	deps := objectsOf[*appsv1.Deployment](objs)
	require.Len(t, deps, 1)
	return deps[0].Spec.Template.Annotations[bootstrapChecksumAnnotation]
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

	// The report clears the condition and sets none.
	require.NotNil(t, report)
	assert.Empty(t, report.Conditions)
	assert.Equal(t, []string{conditions.KgatewayConditionBootstrapPatchApplied}, report.OwnedConditionTypes)
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

	// Rendering the same patches again gives the same checksum; changing them changes it.
	again, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}.render(t)
	require.NoError(t, err)
	assert.Equal(t, checksum(t, objs), checksum(t, again), "identical patches should give an identical checksum")
	changed, _, err := bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), `{stats_flush_interval: 10s}`)}.render(t)
	require.NoError(t, err)
	assert.NotEqual(t, checksum(t, objs), checksum(t, changed), "different patches should change the checksum")
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
			assert.ErrorContains(t, patchErr, tt.wantErr)
			assert.Nil(t, objs)
			assert.Nil(t, report)
		})
	}
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

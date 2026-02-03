package deployer

// This test suite validates helm chart rendering and post-processing with
// overlays (strategic-merge-patch) for managed Gateway deployments.
//
// # Fake Client and Server-Side Apply Semantics
//
// The fake client used in these tests (no need for envtest, which is slower
// and still not as thorough as an e2e test) preserves null values in CRD
// fields marked with x-kubernetes-preserve-unknown-fields, mimicking the
// behavior of `kubectl apply --server-side`. This differs from regular
// client-side `kubectl apply`, which strips null values before sending them to
// the API server.
//
// This means tests here accurately reflect what happens when users apply
// AgentgatewayParameters with `kubectl apply --server-side`, helm 4 in default
// `--server-side` mode, Argo CD with ServerSideApply set to true, etc. If a
// user uses regular `kubectl apply` with null values in overlay fields, the
// nulls will be stripped and the strategic merge patch won't see them. That's
// why our API docs say to prefer using `$patch: delete` instead of null values
// when removing fields. See the API documentation for
// KubernetesResourceOverlay.Spec for details.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	pkgdeployer "github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/version"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func mockVersion(t *testing.T) {
	// Save the original version and restore it after the test
	// This ensures the test uses a fixed version (1.0.0-ci1) regardless of
	// what VERSION was set when compiling the test binary
	originalVersion := version.Version
	version.Version = "1.0.0-ci1"
	t.Cleanup(func() {
		version.Version = originalVersion
	})
}

func TestRenderHelmChart(t *testing.T) {
	mockVersion(t)

	// Create temporary CA certificate file for TLS tests
	caCertContent := `-----BEGIN CERTIFICATE-----
MIICljCCAX4CCQCKSGhvPtMNGzANBgkqhkiG9w0BAQsFADANMQswCQYDVQQGEwJV
UzAeFw0yNDA3MDEwMDAwMDBaFw0yNTA3MDEwMDAwMDBaMA0xCzAJBgNVBAYTAlVT
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1234567890ABCDEFGHIj
klmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ab
cdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ123456
7890abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ
1234567890abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTU
VWXYZ1234567890abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNO
PQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHI
JKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyz1234567890ABC
DEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyz123456
wIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQBtestcertdata
-----END CERTIFICATE-----`
	tmpDir := t.TempDir()
	caCertPath := tmpDir + "/ca.crt"
	err := os.WriteFile(caCertPath, []byte(caCertContent), 0o600)
	require.NoError(t, err)

	// TLS override function for tests that need TLS enabled
	tlsOverride := func(caCertPath string) func(inputs *pkgdeployer.Inputs) pkgdeployer.HelmValuesGenerator {
		return func(inputs *pkgdeployer.Inputs) pkgdeployer.HelmValuesGenerator {
			inputs.ControlPlane.XdsTLS = true
			inputs.ControlPlane.XdsTlsCaPath = caCertPath
			return nil
		}
	}

	// Istio override function for tests that need Istio auto mTLS enabled
	istioOverride := func(inputs *pkgdeployer.Inputs) pkgdeployer.HelmValuesGenerator {
		inputs.IstioAutoMtlsEnabled = true
		return nil
	}

	tests := []HelmTestCase{
		{
			Name:      "basic gateway with default gatewayclass and no gwparams",
			InputFile: "base-gateway",
		},
		{
			Name:      "gateway with replicas GWP via GWC",
			InputFile: "gwc-with-replicas",
		},
		{
			Name:      "gateway with priorityClassName",
			InputFile: "priority-class-name",
		},
		{
			Name:      "gwparams with omitDefaultSecurityContext via GWC",
			InputFile: "omit-default-security-context",
			Validate:  NoSecurityContextValidator(),
		},
		{
			Name:      "gwparams with omitDefaultSecurityContext via GW",
			InputFile: "omit-default-security-context-via-gw",
			Validate:  NoSecurityContextValidator(),
		},
		{
			Name:      "gwparams with stats matcher inclusion",
			InputFile: "stats-matcher-inclusion",
		},
		{
			Name:      "gwparams with stats matcher exclusion",
			InputFile: "stats-matcher-exclusion",
		},
		{
			Name:      "agentgateway",
			InputFile: "agentgateway",
		},
		{
			// Uses $patch: delete for pod-level and null for container-level securityContext
			Name:      "agentgateway omit securityContext via $patch:delete and null AGWP via GWC",
			InputFile: "agentgateway-omitdefaultsecuritycontext",
			Validate:  EmptySecurityContextValidator(),
		},
		{
			// Uses null for pod-level and $patch: delete for container-level securityContext
			Name:      "agentgateway omit securityContext via null and $patch:delete AGWP via GW",
			InputFile: "agentgateway-omitdefaultsecuritycontext-ref-gwp-on-gw",
			Validate:  EmptySecurityContextValidator(),
		},
		{
			Name:      "agentgateway-infrastructure with AgentgatewayParameters",
			InputFile: "agentgateway-infrastructure",
		},
		{
			Name:      "agentgateway-controller-but-custom-gatewayclass",
			InputFile: "agentgateway-controller-but-custom-gatewayclass",
		},
		{
			Name:      "envoy-controller-ignores-agentgateway-class-name",
			InputFile: "envoy-controller-ignores-agentgateway-class-name",
		},
		{
			Name:      "envoy-infrastructure",
			InputFile: "envoy-infrastructure",
		},
		{
			Name:      "envoy dns resolver params",
			InputFile: "envoy-dns-resolver",
		},
		{
			Name:      "envoy dns resolver disable",
			InputFile: "envoy-dns-resolver-zero",
		},
		{
			// The GW parametersRef merges with the GWC parametersRef.
			// GWC has replicas:2, GW has omitDefaultSecurityContext:true.
			// Both settings should appear in the output.
			Name:      "both GWC and GW have parametersRef",
			InputFile: "both-gwc-and-gw-have-params",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "replicas: 2",
					"replicas from GatewayClass params should be preserved when Gateway has omitDefaultSecurityContext")
				assert.NotContains(t, outputYaml, "securityContext",
					"securityContext should be omitted due to Gateway's omitDefaultSecurityContext:true")
			},
		},
		{
			// Like the above, but swap the actual parameters to test the test:
			Name:      "both GWC and GW have parametersRef reversed",
			InputFile: "both-gwc-and-gw-have-params-reversed",
		},
		{
			Name:      "gateway with static IP address",
			InputFile: "loadbalancer-static-ip",
		},
		{
			Name:      "gateway with loadBalancerClass",
			InputFile: "loadbalancer-class",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "loadBalancerClass: service.k8s.aws/nlb",
					"loadBalancerClass should be set on the Service")
				assert.Contains(t, outputYaml, "type: LoadBalancer",
					"Service type should be LoadBalancer")
			},
		},
		{
			Name:      "agentgateway-params-primary",
			InputFile: "agentgateway-params-primary",
		},
		{
			Name:      "agentgateway with full image override",
			InputFile: "agentgateway-image-override",
		},
		{
			Name:      "agentgateway with env vars",
			InputFile: "agentgateway-env",
		},
		{
			Name:      "agentgateway with shutdown configuration",
			InputFile: "agentgateway-shutdown",
		},
		{
			Name:      "agentgateway with Istio configuration",
			InputFile: "agentgateway-istio",
		},
		{
			Name:      "agentgateway with logging format json",
			InputFile: "agentgateway-logging-format",
		},
		{
			Name:      "agentgateway yaml injection",
			InputFile: "agentgateway-yaml-injection",
		},
		{
			Name:      "agentgateway rawConfig with typed config conflict",
			InputFile: "agentgateway-rawconfig-typed-conflict",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "format: text",
					"typed logging.format: text should take precedence over rawConfig's json")
				assert.NotContains(t, outputYaml, "format: json",
					"rawConfig's logging.format: json should be overridden by typed config")
				assert.Contains(t, outputYaml, "jaeger:4317",
					"tracing config from rawConfig should be merged in")
			},
		},
		{
			Name:      "agentgateway rawConfig with binds for direct response",
			InputFile: "agentgateway-rawconfig-binds",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "  config.yaml: |\n    binds:\n",
					"binds config should be present in ConfigMap as a top-level config.yaml key")
				assert.Contains(t, outputYaml, "port: 3000",
					"binds port 3000 should be present")
			},
		},
		{
			Name:      "agentgateway with repository only image override",
			InputFile: "agentgateway-image-repo-only",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.NotContains(t, outputYaml, "imagePullPolicy:",
					"output YAML should not contain imagePullPolicy, allowing k8s to look at the tag to decide")
			},
		},
		{
			// Test merging GWC and GW AgentgatewayParameters.
			Name:      "agentgateway both GWC and GW have parametersRef",
			InputFile: "agentgateway-both-gwc-and-gw-have-params",
		},
		{
			Name:      "agentgateway strategic-merge-patch tests",
			InputFile: "agentgateway-strategic-merge-patch",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				// Deployment overlay metadata applied
				assert.Contains(t, outputYaml, "deployment-overlay-annotation: from-overlay",
					"deployment annotation from overlay should be present")
				assert.Contains(t, outputYaml, "deployment-overlay-label1: from-overlay",
					"deployment label from overlay should be present")

				// $patch: delete on env var RUST_LOG
				assert.NotContains(t, outputYaml, "RUST_LOG",
					"RUST_LOG env var should be deleted via $patch: delete")

				// $patch: replace on volumes - only custom-config volume in volumes list
				// (volumeMounts is a separate list that still has the original mounts)
				assert.Contains(t, outputYaml, "name: my-custom-config",
					"custom configmap from $patch: replace should be present")
				// Verify only one volume exists (the custom one) by checking volumes section structure
				assert.Contains(t, outputYaml, "volumes:\n      - configMap:\n          name: my-custom-config\n        name: custom-config\nstatus:",
					"volumes should be replaced with only custom-config")

				// $patch: replace on service ports
				assert.Contains(t, outputYaml, "port: 80",
					"service port 80 from $patch: replace should be present")
				assert.Contains(t, outputYaml, "port: 443",
					"service port 443 from $patch: replace should be present")
				// The original Gateway listener port 8080 becomes targetPort, not port
				assert.NotContains(t, outputYaml, "port: 8080\n",
					"default port 8080 should be replaced (only exists as targetPort now)")

				// $setElementOrder/args - args reordered
				assert.Contains(t, outputYaml, "- /config/config.yaml\n        - -f",
					"args should be reordered via $setElementOrder")

				// Service overlay annotation
				assert.Contains(t, outputYaml, "service-overlay-annotation: from-overlay",
					"service annotation from overlay should be present")

				// Label nulled to empty string
				assert.Contains(t, outputYaml, `app.kubernetes.io/managed-by: ""`,
					"label should be nulled to empty string")

				// Volume mount added via merge
				assert.Contains(t, outputYaml, "mountPath: /etc/custom-config",
					"custom volumeMount should be added via merge")
			},
		},
		{
			Name:      "agentgateway AGWP with pod scheduling fields",
			InputFile: "agentgateway-agwp-pod-scheduling",
		},
		{
			Name:      "agentgateway with static IP address via overlay",
			InputFile: "agentgateway-loadbalancer-static-ip",
		},
		{
			Name:      "agentgateway GKE with subsetting and external static IP",
			InputFile: "agentgateway-gke-subsetting-static-ip",
		},
		{
			Name:      "agentgateway with PodDisruptionBudget overlay",
			InputFile: "agentgateway-pdb-overlay",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "kind: PodDisruptionBudget",
					"PDB should be created when podDisruptionBudget overlay is specified")
				assert.Contains(t, outputYaml, "pdb-label: from-overlay",
					"PDB should have label from overlay")
				assert.Contains(t, outputYaml, "pdb-annotation: from-overlay",
					"PDB should have annotation from overlay")
				assert.Contains(t, outputYaml, "minAvailable: 1",
					"PDB should have minAvailable from overlay spec")
			},
		},
		{
			Name:      "agentgateway with HorizontalPodAutoscaler overlay",
			InputFile: "agentgateway-hpa-overlay",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "kind: HorizontalPodAutoscaler",
					"HPA should be created when horizontalPodAutoscaler overlay is specified")
				assert.Contains(t, outputYaml, "hpa-label: from-overlay",
					"HPA should have label from overlay")
				assert.Contains(t, outputYaml, "hpa-annotation: from-overlay",
					"HPA should have annotation from overlay")
				assert.Contains(t, outputYaml, "minReplicas: 2",
					"HPA should have minReplicas from overlay spec")
				assert.Contains(t, outputYaml, "maxReplicas: 10",
					"HPA should have maxReplicas from overlay spec")
				assert.Contains(t, outputYaml, "averageUtilization: 80",
					"HPA should have CPU utilization target from overlay spec")
			},
		},
		// Envoy (kgateway) overlay test cases
		{
			Name:      "envoy with PodDisruptionBudget overlay",
			InputFile: "envoy-pdb-overlay",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "kind: PodDisruptionBudget",
					"PDB should be created when podDisruptionBudget overlay is specified")
				assert.Contains(t, outputYaml, "pdb-label: from-overlay",
					"PDB should have label from overlay")
				assert.Contains(t, outputYaml, "pdb-annotation: from-overlay",
					"PDB should have annotation from overlay")
				assert.Contains(t, outputYaml, "minAvailable: 1",
					"PDB should have minAvailable from overlay spec")
			},
		},
		{
			Name:      "envoy with HorizontalPodAutoscaler overlay",
			InputFile: "envoy-hpa-overlay",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "kind: HorizontalPodAutoscaler",
					"HPA should be created when horizontalPodAutoscaler overlay is specified")
				assert.Contains(t, outputYaml, "hpa-label: from-overlay",
					"HPA should have label from overlay")
				assert.Contains(t, outputYaml, "hpa-annotation: from-overlay",
					"HPA should have annotation from overlay")
				assert.Contains(t, outputYaml, "minReplicas: 2",
					"HPA should have minReplicas from overlay spec")
				assert.Contains(t, outputYaml, "maxReplicas: 10",
					"HPA should have maxReplicas from overlay spec")
				assert.Contains(t, outputYaml, "averageUtilization: 80",
					"HPA should have CPU utilization target from overlay spec")
			},
		},
		{
			Name:      "envoy with VerticalPodAutoscaler overlay",
			InputFile: "envoy-vpa-overlay",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "kind: VerticalPodAutoscaler",
					"VPA should be created when verticalPodAutoscaler overlay is specified")
				assert.Contains(t, outputYaml, "vpa-label: from-overlay",
					"VPA should have label from overlay")
				assert.Contains(t, outputYaml, "vpa-annotation: from-overlay",
					"VPA should have annotation from overlay")
				assert.Contains(t, outputYaml, "updateMode: Auto",
					"VPA should have updateMode from overlay spec")
				assert.Contains(t, outputYaml, "containerName: envoy",
					"VPA should have containerName from overlay spec")
			},
		},
		{
			Name:      "envoy strategic-merge-patch tests",
			InputFile: "envoy-strategic-merge-patch",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				// Deployment overlay metadata applied
				assert.Contains(t, outputYaml, "deployment-overlay-annotation: from-overlay",
					"deployment annotation from overlay should be present")
				assert.Contains(t, outputYaml, "deployment-overlay-label1: from-overlay",
					"deployment label from overlay should be present")

				// Service overlay metadata applied
				assert.Contains(t, outputYaml, "service-overlay-annotation: from-overlay",
					"service annotation from overlay should be present")

				// ServiceAccount overlay metadata applied
				assert.Contains(t, outputYaml, "sa-overlay-annotation: from-overlay",
					"serviceaccount annotation from overlay should be present")
				assert.Contains(t, outputYaml, "sa-overlay-label: from-overlay",
					"serviceaccount label from overlay should be present")

				// $patch: replace on volumes - only custom-config volume in volumes list
				assert.Contains(t, outputYaml, "name: my-custom-config",
					"custom configmap from $patch: replace should be present")
			},
		},
		{
			Name:      "envoy both GWC and GW have overlays",
			InputFile: "envoy-both-gwc-and-gw-have-overlays",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				// GWC annotations/labels should be present
				assert.Contains(t, outputYaml, "gwc-annotation: from-gatewayclass",
					"GWC annotation should be present")
				assert.Contains(t, outputYaml, "gwc-label: from-gatewayclass",
					"GWC label should be present")

				// GW annotations/labels should be present
				assert.Contains(t, outputYaml, "gw-annotation: from-gateway",
					"GW annotation should be present")
				assert.Contains(t, outputYaml, "gw-label: from-gateway",
					"GW label should be present")

				// Shared annotation/label should be from Gateway (applied second, overrides GWC)
				assert.Contains(t, outputYaml, "shared-annotation: from-gateway",
					"shared annotation should be from Gateway (overrides GWC)")
				assert.Contains(t, outputYaml, "shared-label: from-gateway",
					"shared label should be from Gateway (overrides GWC)")

				// PDB from GWC should be present
				assert.Contains(t, outputYaml, "kind: PodDisruptionBudget",
					"PDB from GWC should be created")
				assert.Contains(t, outputYaml, "pdb-source: gatewayclass",
					"PDB should have label from GWC overlay")

				// HPA from GW should be present
				assert.Contains(t, outputYaml, "kind: HorizontalPodAutoscaler",
					"HPA from GW should be created")
				assert.Contains(t, outputYaml, "hpa-source: gateway",
					"HPA should have label from GW overlay")
			},
		},
		{
			Name:      "envoy with PDB and autoscalers (HPA, VPA)",
			InputFile: "envoy-all-autoscalers",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "kind: PodDisruptionBudget",
					"PDB should be created")
				assert.Contains(t, outputYaml, "kind: HorizontalPodAutoscaler",
					"HPA should be created")
				assert.Contains(t, outputYaml, "kind: VerticalPodAutoscaler",
					"VPA should be created")
				assert.Contains(t, outputYaml, "resource-type: pdb",
					"PDB should have resource-type label")
				assert.Contains(t, outputYaml, "resource-type: hpa",
					"HPA should have resource-type label")
				assert.Contains(t, outputYaml, "resource-type: vpa",
					"VPA should have resource-type label")
			},
		},
		{
			// This test proves the documented overlay ordering:
			// 1. GatewayClass overlay is applied first
			// 2. Gateway overlay is applied second (overrides GatewayClass values)
			// Both metadata (labels/annotations) and spec fields demonstrate this ordering.
			Name:      "envoy overlay ordering - GWC first then GW",
			InputFile: "envoy-overlay-ordering",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()

				// Test 1: Metadata ordering
				// The shared "overlay-source" label/annotation should be "from-gateway" (GW wins)
				assert.Contains(t, outputYaml, "overlay-source: from-gateway",
					"shared label/annotation should be from Gateway (applied second, overrides GWC)")
				assert.NotContains(t, outputYaml, "overlay-source: from-gatewayclass",
					"GatewayClass value should be overridden by Gateway")

				// Both GWC-only and GW-only values should be present (merged)
				assert.Contains(t, outputYaml, "gwc-only-annotation: present",
					"GWC-only annotation should be preserved")
				assert.Contains(t, outputYaml, "gwc-only-label: present",
					"GWC-only label should be preserved")
				assert.Contains(t, outputYaml, "gw-only-annotation: present",
					"GW-only annotation should be present")
				assert.Contains(t, outputYaml, "gw-only-label: present",
					"GW-only label should be present")

				// Test 2: Spec ordering
				// GWC sets terminationGracePeriodSeconds: 29
				// GW sets terminationGracePeriodSeconds: 59
				// GW should win (applied second)
				assert.Contains(t, outputYaml, "terminationGracePeriodSeconds: 59",
					"terminationGracePeriodSeconds should be 59 from Gateway (overrides GWC's 29)")
				assert.NotContains(t, outputYaml, "terminationGracePeriodSeconds: 29",
					"GWC's terminationGracePeriodSeconds: 29 should be overridden")

				// Service overlay ordering - GW should win
				assert.Contains(t, outputYaml, "overlay-source: from-gateway",
					"service annotation should be from Gateway")
			},
		},
		{
			// This test demonstrates:
			// 1. Configs vs Overlays precedence: overlays are applied AFTER configs
			//    - GWC config: replicas: 2
			//    - GWC overlay: replicas: 5
			//    - Result: replicas: 5 (overlay wins)
			// 2. GatewayClass GWP vs Gateway GWP merging:
			//    - GWC config: service.type: ClusterIP
			//    - GW config: service.type: LoadBalancer
			//    - Result: LoadBalancer (Gateway GWP wins)
			// 3. Overlay merging between GWC and GW:
			//    - shared-annotation: GW wins with "from-gateway"
			//    - unique annotations from both are preserved
			Name:      "envoy configs applied first then overlays on top",
			InputFile: "envoy-configs-and-overlays",
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()

				// 1. Overlays override configs within the same GWP
				// GWC config sets replicas: 2, GWC overlay sets replicas: 5
				assert.Contains(t, outputYaml, "replicas: 5",
					"overlay replicas: 5 should override config replicas: 2")
				assert.NotContains(t, outputYaml, "replicas: 2",
					"config replicas: 2 should be overridden by overlay")

				// 2. Gateway GWP configs override GatewayClass GWP configs
				// GWC config: ClusterIP, GW config: LoadBalancer
				assert.Contains(t, outputYaml, "type: LoadBalancer",
					"Gateway GWP service.type should override GatewayClass GWP")

				// 3a. Deployment overlay merging - shared annotations: GW wins
				assert.Contains(t, outputYaml, "shared-annotation: from-gateway",
					"Gateway overlay should win for shared-annotation")
				assert.NotContains(t, outputYaml, "shared-annotation: from-gatewayclass",
					"GatewayClass value for shared-annotation should be overridden")

				// 3b. Deployment overlay merging - unique annotations preserved
				assert.Contains(t, outputYaml, "gwc-only-annotation: from-gatewayclass",
					"GatewayClass-only annotation should be preserved")
				assert.Contains(t, outputYaml, "gw-only-annotation: from-gateway",
					"Gateway-only annotation should be present")

				// 3c. Same for labels
				assert.Contains(t, outputYaml, "shared-label: from-gateway",
					"Gateway overlay should win for shared-label")
				assert.Contains(t, outputYaml, "gwc-only-label: from-gatewayclass",
					"GatewayClass-only label should be preserved")
				assert.Contains(t, outputYaml, "gw-only-label: from-gateway",
					"Gateway-only label should be present")

				// 4. Service overlay merging
				assert.Contains(t, outputYaml, "shared-service-annotation: from-gateway",
					"Gateway service overlay should win for shared annotation")
				assert.Contains(t, outputYaml, "gwc-service-annotation: from-gatewayclass",
					"GatewayClass service annotation should be preserved")
				assert.Contains(t, outputYaml, "gw-service-annotation: from-gateway",
					"Gateway service annotation should be present")
			},
		},
		{
			// This test demonstrates the recommended approach for adding sidecars:
			// - Use sdsContainer for TLS certificate handling (built-in SDS, requires Istio)
			// - Use deploymentOverlay for other custom sidecars
			Name:                        "envoy with SDS container and custom sidecar via overlay",
			InputFile:                   "envoy-sds-and-custom-sidecar",
			HelmValuesGeneratorOverride: istioOverride,
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()

				// SDS container should be present with custom image (requires Istio enabled)
				assert.Contains(t, outputYaml, "name: sds",
					"SDS container should be present when Istio is enabled")
				assert.Contains(t, outputYaml, "ghcr.io/kgateway-dev/sds:v1.0.0",
					"SDS container should use custom image")

				// Custom sidecar added via overlay should be present
				assert.Contains(t, outputYaml, "name: my-sidecar",
					"custom sidecar from overlay should be present")
				assert.Contains(t, outputYaml, "image: my-sidecar:latest",
					"custom sidecar should use specified image")

				// Main proxy container should still be present
				assert.Contains(t, outputYaml, "name: kgateway-proxy",
					"main proxy container should be present")

				// Istio proxy container should also be present
				assert.Contains(t, outputYaml, "name: istio-proxy",
					"istio-proxy container should be present when Istio is enabled")
			},
		},
		// TLS test cases
		{
			Name:                        "basic gateway with TLS enabled",
			InputFile:                   "base-gateway-tls",
			HelmValuesGeneratorOverride: tlsOverride(caCertPath),
		},
		{
			Name:                        "agentgateway with TLS enabled",
			InputFile:                   "agentgateway-tls",
			HelmValuesGeneratorOverride: tlsOverride(caCertPath),
		},
		{
			// Custom configmap name via AgentgatewayParameters deployment overlay:
			Name:      "agentgateway with custom configmap name via overlay",
			InputFile: "agentgateway-custom-configmap",
		},
		{
			Name:      "agentgateway with Gateway.spec.addresses",
			InputFile: "agentgateway-gateway-addresses",
		},
		{
			Name:                        "gateway with istio enabled",
			InputFile:                   "istio-enabled",
			HelmValuesGeneratorOverride: istioOverride,
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "name: sds",
					"sds container should be present when istio is enabled")
				assert.Contains(t, outputYaml, "name: istio-proxy",
					"istio-proxy container should be present when istio is enabled")
				assert.Contains(t, outputYaml, "ISTIO_MTLS_SDS_ENABLED",
					"ISTIO_MTLS_SDS_ENABLED env var should be present")
				assert.Contains(t, outputYaml, "name: istio-certs",
					"istio-certs volume should be present")
			},
		},
		{
			Name:                        "waypoint gateway with istio enabled",
			InputFile:                   "istio-enabled-waypoint",
			HelmValuesGeneratorOverride: istioOverride,
			Validate: func(t *testing.T, outputYaml string) {
				t.Helper()
				assert.Contains(t, outputYaml, "name: sds",
					"sds container should be present when istio is enabled")
				assert.Contains(t, outputYaml, "name: istio-proxy",
					"istio-proxy container should be present when istio is enabled")
				// Waypoint-specific: ClusterIP service type
				assert.Contains(t, outputYaml, "type: ClusterIP",
					"waypoint should have ClusterIP service type")
				// Waypoint-specific: port 15008 for HBONE
				assert.Contains(t, outputYaml, "port: 15008",
					"waypoint should have port 15008 for HBONE")
			},
		},
		{
			Name:      "gateway with name exactly 63 characters",
			InputFile: "long-gateway-name-exactly-63-chars",
		},
		{
			Name:      "gateway with name over 63 characters",
			InputFile: "long-gateway-name-over-63-chars",
		},
		{
			Name:      "agentgateway with name exactly 63 characters",
			InputFile: "agentgateway-long-gateway-name-exactly-63-chars",
		},
		{
			Name:      "agentgateway with name over 63 characters",
			InputFile: "agentgateway-long-gateway-name-over-63-chars",
		},
	}

	tester := DeployerTester{
		ControllerName:    wellknown.DefaultGatewayControllerName,
		AgwControllerName: wellknown.DefaultAgwControllerName,
		ClassName:         wellknown.DefaultGatewayClassName,
		WaypointClassName: wellknown.DefaultWaypointClassName,
		AgwClassName:      wellknown.DefaultAgwClassName,
	}

	dir := fsutils.MustGetThisDir()
	scheme := schemes.GatewayScheme()
	crdDir := filepath.Join(testutils.GitRootDirectory(), testutils.CRDPath)

	VerifyAllYAMLFilesReferenced(t, filepath.Join(dir, "testdata"), tests)
	VerifyAllEnvoyBootstrapAreValid(t, filepath.Join(dir, "testdata"))

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			fakeClient := fake.NewClient(t, tester.GetObjects(t, tt, scheme, dir, crdDir)...)
			tester.RunHelmChartTest(t, tt, scheme, dir, crdDir, fakeClient)
		})
	}
}

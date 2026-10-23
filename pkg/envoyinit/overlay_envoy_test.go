package envoyinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// These tests run the envoy-wrapper image with a branch-built envoyinit and the bootstrap
// the deployer renders for a default Gateway, then read the running Envoy's admin API to
// show how a user-supplied partial bootstrap combines with the kgateway one.

const (
	// renderedGatewayGolden is deployer golden output for a Gateway with default parameters;
	// it holds the bootstrap ConfigMap that the proxy mounts at /etc/envoy.
	renderedGatewayGolden = "../../test/deployer/testdata/base-gateway-out.yaml"
	envoyImageFile        = "../validator/default_envoy_image.txt"

	kgatewayAdminPort = 19000
	overlayAdminPort  = 19001
	overlayMountDir   = "/etc/envoy-overlay"
	overlayFileName   = "runtime-overlay.yaml"

	fragmentFeature = "envoy.reloadable_features.http_reject_path_with_fragment"
	// edsCacheFeature is also set to true by the kgateway bootstrap's static_layer.
	edsCacheFeature = "envoy.restart_features.use_eds_cache_for_ads"
)

// testOverlay sets a runtime key kgateway leaves alone, a runtime key and a scalar
// (admin port) that kgateway also sets, and adds an entry to a repeated field.
var testOverlay = fmt.Sprintf(`
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
layered_runtime:
  layers:
  - name: custom_runtime_overrides
    static_layer:
      %s: false
      %s: false
static_resources:
  clusters:
  - name: overlay_cluster
    type: STATIC
    load_assignment:
      cluster_name: overlay_cluster
      endpoints:
      - lb_endpoints:
        - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: 1 } } }
`, overlayAdminPort, fragmentFeature, edsCacheFeature)

// TestEnvoyExtraArgsConfigPathMergesUnderBootstrap covers the approach that needs no code
// change: mount the overlay and pass `-c <overlay>` in envoyContainer.extraArgs. envoyinit
// execs `envoy --config-yaml <kgateway bootstrap> <args>`, and Envoy loads --config-path
// first and merges --config-yaml over it, so kgateway's bootstrap wins every conflict.
func TestEnvoyExtraArgsConfigPathMergesUnderBootstrap(t *testing.T) {
	proxy := startProxy(t, nil, "-c", overlayMountDir+"/"+overlayFileName)

	rt := proxy.runtime(t, kgatewayAdminPort)
	assert.Equal(t, []string{"custom_runtime_overrides", "static_layer", "admin_layer"}, rt.Layers,
		"overlay layers are appended before the kgateway layers")
	assert.Equal(t, "false", rt.Entries[fragmentFeature].FinalValue,
		"a runtime key kgateway does not set takes the overlay value")
	assert.Equal(t, runtimeEntry{FinalValue: "true", LayerValues: []string{"false", "true", ""}}, rt.Entries[edsCacheFeature],
		"a runtime key kgateway also sets keeps the kgateway value")

	bootstrap := proxy.bootstrap(t, kgatewayAdminPort)
	assert.Equal(t, float64(kgatewayAdminPort), bootstrap.adminPort(),
		"a scalar kgateway also sets keeps the kgateway value")
	assert.Equal(t, append([]string{"overlay_cluster"}, renderedStaticClusterNames(t)...), bootstrap.staticClusterNames(t),
		"repeated fields are concatenated, overlay entries first")

	proxy.assertAdminLayerWins(t, kgatewayAdminPort)
}

// TestEnvoyExtraArgsCannotPassSecondConfigYaml shows why the overlay cannot win without
// envoyinit's help: envoyinit already passes --config-yaml, and Envoy rejects a repeat.
func TestEnvoyExtraArgsCannotPassSecondConfigYaml(t *testing.T) {
	out, err := runProxy(t, runProxyOpts{}, "--config-yaml", testOverlay)
	require.Error(t, err, "envoy should refuse to start")
	assert.Contains(t, out, "(--config-yaml) -- Argument already set!")
}

// TestEnvoyOverlayConfMergesOverBootstrap covers the alternative: set OVERLAY_CONF on the
// proxy container and envoyinit merges the overlay over the kgateway bootstrap, keeping
// the admin runtime layer last.
func TestEnvoyOverlayConfMergesOverBootstrap(t *testing.T) {
	proxy := startProxy(t, map[string]string{overlayConfigPathEnv: overlayMountDir + "/" + overlayFileName})

	rt := proxy.runtime(t, overlayAdminPort)
	assert.Equal(t, []string{"static_layer", "custom_runtime_overrides", "admin_layer"}, rt.Layers,
		"overlay layers sit after the kgateway static layer and before the admin layer")
	assert.Equal(t, "false", rt.Entries[fragmentFeature].FinalValue,
		"a runtime key kgateway does not set takes the overlay value")
	assert.Equal(t, runtimeEntry{FinalValue: "false", LayerValues: []string{"true", "false", ""}}, rt.Entries[edsCacheFeature],
		"a runtime key kgateway also sets takes the overlay value")

	bootstrap := proxy.bootstrap(t, overlayAdminPort)
	assert.Equal(t, float64(overlayAdminPort), bootstrap.adminPort(),
		"a scalar kgateway also sets takes the overlay value")
	assert.Equal(t, append(renderedStaticClusterNames(t), "overlay_cluster"), bootstrap.staticClusterNames(t),
		"repeated fields are concatenated, overlay entries last")

	proxy.assertAdminLayerWins(t, overlayAdminPort)
}

// TestEnvoyOverlayConfFailsFastOnBadOverlay checks that a broken overlay stops the proxy
// with a clear error instead of starting Envoy without it.
func TestEnvoyOverlayConfFailsFastOnBadOverlay(t *testing.T) {
	out, err := runProxy(t, runProxyOpts{
		overlay: "layerd_runtime: {}\n",
		env:     map[string]string{overlayConfigPathEnv: overlayMountDir + "/" + overlayFileName},
	})
	require.Error(t, err, "envoyinit should refuse to start")
	assert.Contains(t, out, "initializer failed: failed to unmarshal bootstrap overlay")
}

type proxyContainer struct {
	name string
}

type runtimeEntry struct {
	FinalValue  string   `json:"final_value"`
	LayerValues []string `json:"layer_values"`
}

type runtimeDump struct {
	Layers  []string                `json:"layers"`
	Entries map[string]runtimeEntry `json:"entries"`
}

// runtime returns the parsed /runtime admin output.
func (p proxyContainer) runtime(t *testing.T, adminPort int) runtimeDump {
	t.Helper()
	var rt runtimeDump
	require.NoError(t, json.Unmarshal([]byte(p.admin(t, adminPort, "/runtime", false)), &rt))
	return rt
}

// assertAdminLayerWins shows /runtime_modify still overrides the overlay, i.e. the admin
// layer is still the last layer.
func (p proxyContainer) assertAdminLayerWins(t *testing.T, adminPort int) {
	t.Helper()
	p.admin(t, adminPort, "/runtime_modify?"+fragmentFeature+"=true", true)
	assert.Equal(t, "true", p.runtime(t, adminPort).Entries[fragmentFeature].FinalValue,
		"/runtime_modify overrides the overlay")
}

type bootstrapDump map[string]any

// bootstrap returns the bootstrap Envoy is running with, from /config_dump.
func (p proxyContainer) bootstrap(t *testing.T, adminPort int) bootstrapDump {
	t.Helper()
	var dump struct {
		Configs []map[string]any `json:"configs"`
	}
	require.NoError(t, json.Unmarshal([]byte(p.admin(t, adminPort, "/config_dump", false)), &dump))
	for _, c := range dump.Configs {
		if strings.HasSuffix(fmt.Sprint(c["@type"]), ".BootstrapConfigDump") {
			return c["bootstrap"].(map[string]any)
		}
	}
	require.FailNow(t, "no BootstrapConfigDump in /config_dump")
	return nil
}

func (b bootstrapDump) adminPort() any {
	return dig(b, "admin", "address", "socket_address", "port_value")
}

func (b bootstrapDump) staticClusterNames(t *testing.T) []string {
	t.Helper()
	clusters, ok := dig(b, "static_resources", "clusters").([]any)
	require.True(t, ok, "bootstrap has static clusters")
	var names []string
	for _, c := range clusters {
		names = append(names, fmt.Sprint(c.(map[string]any)["name"]))
	}
	return names
}

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[k]
	}
	return cur
}

// admin calls the Envoy admin API from inside the container, where it listens on loopback.
func (p proxyContainer) admin(t *testing.T, adminPort int, path string, post bool) string {
	t.Helper()
	args := []string{"exec", p.name, "wget", "-qO-"}
	if post {
		args = append(args, "--post-data=")
	}
	args = append(args, fmt.Sprintf("http://127.0.0.1:%d%s", adminPort, path))
	out, err := docker(args...).CombinedOutput()
	require.NoError(t, err, "admin %s: %s", path, out)
	return string(out)
}

// startProxy starts the proxy container detached and waits for the admin API on either
// candidate port.
func startProxy(t *testing.T, env map[string]string, extraArgs ...string) proxyContainer {
	t.Helper()
	p := proxyContainer{name: fmt.Sprintf("envoyinit-overlay-%d", time.Now().UnixNano())}
	out, err := runProxy(t, runProxyOpts{name: p.name, env: env}, extraArgs...)
	require.NoError(t, err, "docker run: %s", out)
	t.Cleanup(func() { _ = docker("rm", "-f", p.name).Run() })

	deadline := time.Now().Add(30 * time.Second)
	for {
		for _, port := range []int{kgatewayAdminPort, overlayAdminPort} {
			// /server_info answers 200 as soon as the admin listener is up, even while
			// Envoy waits for the (absent) control plane.
			url := fmt.Sprintf("http://127.0.0.1:%d/server_info", port)
			if docker("exec", p.name, "wget", "-qO/dev/null", url).Run() == nil {
				return p
			}
		}
		state, _ := docker("inspect", "-f", "{{.State.Running}}", p.name).Output()
		if strings.TrimSpace(string(state)) != "true" || time.Now().After(deadline) {
			logs, _ := docker("logs", p.name).CombinedOutput()
			require.FailNow(t, "envoy admin API did not come up", "logs:\n%s", logs)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

type runProxyOpts struct {
	// name runs the container detached under this name; empty runs it in the foreground.
	name string
	// overlay is the content of the mounted overlay file; empty uses testOverlay.
	overlay string
	env     map[string]string
}

// runProxy runs the envoy-wrapper image the way the proxy Deployment does: the image
// entrypoint, the kgateway-managed args followed by extraArgs, the rendered bootstrap at
// /etc/envoy, and the overlay mounted at overlayMountDir.
func runProxy(t *testing.T, opts runProxyOpts, extraArgs ...string) (string, error) {
	t.Helper()
	dir := proxyFiles(t, opts.overlay)
	args := []string{"run"}
	if opts.name != "" {
		args = append(args, "-d", "--name", opts.name)
	} else {
		args = append(args, "--rm")
	}
	args = append(args,
		"-e", "POD_NAME=gw-0",
		"-e", "POD_NAMESPACE=default",
		"-v", filepath.Join(dir, "envoy")+":/etc/envoy:ro",
		"-v", filepath.Join(dir, "tokens")+":/var/run/secrets/tokens:ro",
		"-v", filepath.Join(dir, "overlay")+":"+overlayMountDir+":ro",
		"-v", envoyinitBinary(t)+":/usr/local/bin/envoyinit:ro",
	)
	for k, v := range opts.env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, envoyImage(t),
		"--disable-hot-restart", "--service-node", "gw-0.default", "--log-level", "info")
	args = append(args, extraArgs...)

	var out bytes.Buffer
	cmd := docker(args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// proxyFiles writes the volumes the proxy container mounts.
func proxyFiles(t *testing.T, overlay string) string {
	t.Helper()
	if overlay == "" {
		overlay = testOverlay
	}
	dir := t.TempDir()
	cm := renderedBootstrapConfigMap(t)
	files := map[string]string{
		"tokens/xds-token":           "test-token",
		"overlay/" + overlayFileName: overlay,
	}
	for k, v := range cm.Data {
		files["envoy/"+k] = v
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		// World-readable because the image runs as a non-root user.
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644)) //nolint:gosec // G306: test fixture read by the container user
	}
	require.NoError(t, os.Chmod(dir, 0o755)) //nolint:gosec // G302: see above
	return dir
}

func renderedBootstrapConfigMap(t *testing.T) corev1.ConfigMap {
	t.Helper()
	golden, err := os.ReadFile(renderedGatewayGolden)
	require.NoError(t, err)
	for doc := range strings.SplitSeq(string(golden), "\n---\n") {
		var cm corev1.ConfigMap
		if yaml.Unmarshal([]byte(doc), &cm) == nil && cm.Kind == "ConfigMap" && cm.Data["envoy.yaml"] != "" {
			return cm
		}
	}
	require.FailNow(t, "no bootstrap ConfigMap in "+renderedGatewayGolden)
	return corev1.ConfigMap{}
}

func envoyImage(t *testing.T) string {
	t.Helper()
	img, err := os.ReadFile(envoyImageFile)
	require.NoError(t, err)
	return strings.TrimSpace(string(img))
}

var buildEnvoyinit = sync.OnceValues(func() (string, error) {
	arch, err := docker("version", "-f", "{{.Server.Arch}}").Output()
	if err != nil {
		return "", fmt.Errorf("docker version: %w", err)
	}
	dir, err := os.MkdirTemp("", "envoyinit-overlay-test")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "envoyinit")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/envoyinit")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+strings.TrimSpace(string(arch)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build envoyinit: %w: %s", err, out)
	}
	return bin, os.Chmod(dir, 0o755) //nolint:gosec // G302: mounted into the container
})

// envoyinitBinary cross-compiles envoyinit from this branch for the docker daemon's
// architecture, so the tests exercise the current code rather than the image's copy.
func envoyinitBinary(t *testing.T) string {
	t.Helper()
	bin, err := buildEnvoyinit()
	require.NoError(t, err)
	return bin
}

// renderedStaticClusterNames returns the static cluster names in the kgateway bootstrap.
func renderedStaticClusterNames(t *testing.T) []string {
	t.Helper()
	var b bootstrapDump
	require.NoError(t, yaml.Unmarshal([]byte(renderedBootstrapConfigMap(t).Data["envoy.yaml"]), &b))
	return b.staticClusterNames(t)
}

func docker(args ...string) *exec.Cmd {
	return exec.Command("docker", args...) //nolint:gosec // G204: test-controlled docker args
}

package envoyinit

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/protoutils"
)

const testCAPath = "/etc/ssl/certs/ca-certificates.crt"

// systemCASecret is the secret addSystemCASecret appends, as a generic map.
var systemCASecret = map[string]any{
	"name": "SYSTEM_CA_CERT",
	"validation_context": map[string]any{
		"trusted_ca": map[string]any{"filename": testCAPath},
	},
}

// decode parses YAML into a generic map for comparison.
func decode(t *testing.T, in []byte) map[string]any {
	t.Helper()

	var out map[string]any
	if err := yaml.Unmarshal(in, &out); err != nil {
		t.Fatalf("failed to decode %q: %v", in, err)
	}
	return out
}

func TestAddSystemCASecretAppendsToSecrets(t *testing.T) {
	g := NewWithT(t)

	// A bootstrap with an existing secret and a static cluster.
	in := []byte(`
static_resources:
  secrets:
  - name: existing
    validation_context:
      trusted_ca: {filename: /etc/existing.crt}
  clusters:
  - name: xds_cluster
    connect_timeout: 5s
`)

	// The secret is appended after the existing one, and the cluster is untouched.
	out, err := addSystemCASecret(in, testCAPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(decode(t, out)).To(Equal(map[string]any{
		"static_resources": map[string]any{
			"secrets": []any{
				map[string]any{
					"name": "existing",
					"validation_context": map[string]any{
						"trusted_ca": map[string]any{"filename": "/etc/existing.crt"},
					},
				},
				systemCASecret,
			},
			"clusters": []any{
				map[string]any{"name": "xds_cluster", "connect_timeout": "5s"},
			},
		},
	}))

	// The result still parses as a Bootstrap, with the secret Envoy expects.
	var bs envoybootstrapv3.Bootstrap
	g.Expect(protoutils.UnmarshalYaml(out, &bs)).To(Succeed())
	secrets := bs.GetStaticResources().GetSecrets()
	g.Expect(secrets).To(HaveLen(2))
	g.Expect(secrets[1].GetName()).To(Equal("SYSTEM_CA_CERT"))
	g.Expect(secrets[1].GetValidationContext().GetTrustedCa().GetFilename()).To(Equal(testCAPath))
}

func TestAddSystemCASecretCreatesStaticResources(t *testing.T) {
	g := NewWithT(t)

	// A bootstrap without static_resources gets one holding only the secret.
	out, err := addSystemCASecret([]byte("node: {id: proxy}\n"), testCAPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(decode(t, out)).To(Equal(map[string]any{
		"node":             map[string]any{"id": "proxy"},
		"static_resources": map[string]any{"secrets": []any{systemCASecret}},
	}))

	// An empty bootstrap gets the same.
	out, err = addSystemCASecret(nil, testCAPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(decode(t, out)).To(Equal(map[string]any{
		"static_resources": map[string]any{"secrets": []any{systemCASecret}},
	}))
}

func TestAddSystemCASecretUsesCamelCase(t *testing.T) {
	g := NewWithT(t)

	// A bootstrap that spells the field staticResources keeps that spelling, so the
	// field is not set twice.
	out, err := addSystemCASecret([]byte("staticResources:\n  clusters: []\n"), testCAPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(decode(t, out)).To(Equal(map[string]any{
		"staticResources": map[string]any{
			"clusters": []any{},
			"secrets":  []any{systemCASecret},
		},
	}))
}

func TestAddSystemCASecretKeepsUnknownFieldsAndNumbers(t *testing.T) {
	g := NewWithT(t)

	// A field no Envoy proto has, an unregistered extension type, and an integer too
	// large for a float64 all keep their values, because the bootstrap is never parsed
	// as a proto and integers are not rounded through float64.
	in := []byte(`
field_from_a_newer_envoy: 9007199254740993
bootstrap_extensions:
- name: custom
  typed_config:
    "@type": type.googleapis.com/example.NotRegistered
    value: 1
`)
	out, err := addSystemCASecret(in, testCAPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring("field_from_a_newer_envoy: 9007199254740993\n"))
	g.Expect(string(out)).To(ContainSubstring("'@type': type.googleapis.com/example.NotRegistered"))
}

func TestAddSystemCASecretRejectsWrongShapes(t *testing.T) {
	g := NewWithT(t)

	// static_resources must be a mapping.
	_, err := addSystemCASecret([]byte("static_resources: [a]\n"), testCAPath)
	g.Expect(err).To(MatchError("the bootstrap's static_resources is not a mapping"))

	// secrets must be a list.
	_, err = addSystemCASecret([]byte("staticResources:\n  secrets: {a: b}\n"), testCAPath)
	g.Expect(err).To(MatchError("the bootstrap's staticResources.secrets is not a list"))

	// The bootstrap must be a mapping.
	_, err = addSystemCASecret([]byte("- a\n"), testCAPath)
	g.Expect(err).To(MatchError(ContainSubstring("the bootstrap is not a YAML mapping")))
}

func TestWaitForFilePresent(t *testing.T) {
	g := NewWithT(t)

	// Write the bootstrap file before waiting.
	path := filepath.Join(t.TempDir(), "envoy-abc.yaml")
	g.Expect(os.WriteFile(path, []byte("node: {}"), 0o600)).To(Succeed())

	// The wait returns without polling or logging.
	var logs []string
	waitForFile(path, time.Hour, os.Stat, func(format string, _ ...any) { logs = append(logs, format) })
	g.Expect(logs).To(BeEmpty(), "a present file should not be waited for")
}

func TestWaitForFileAppearsLater(t *testing.T) {
	g := NewWithT(t)

	// Report the file missing for the first three checks.
	path := "/etc/envoy/envoy-abc.yaml"
	calls := 0
	stat := func(name string) (fs.FileInfo, error) {
		calls++
		g.Expect(name).To(Equal(path))
		if calls <= 3 {
			return nil, fs.ErrNotExist
		}
		return nil, nil
	}

	// The wait returns once the file is found, logging the start and the end.
	var logs []string
	waitForFile(path, time.Millisecond, stat, func(format string, _ ...any) { logs = append(logs, format) })
	g.Expect(calls).To(Equal(4), "the wait should check until the file is found")
	g.Expect(logs).To(HaveLen(2), "the wait should log when it starts and when the file appears")
}

func TestWaitForFileOtherError(t *testing.T) {
	g := NewWithT(t)

	// Fail the check with an error other than a missing file.
	calls := 0
	stat := func(string) (fs.FileInfo, error) {
		calls++
		return nil, fs.ErrPermission
	}

	// The wait returns at once, leaving the read to report the error.
	waitForFile("/etc/envoy/envoy-abc.yaml", time.Hour, stat, func(string, ...any) {})
	g.Expect(calls).To(Equal(1), "only a missing file should be waited for")
}

func TestWaitForInputConfigWithoutEnv(t *testing.T) {
	g := NewWithT(t)

	// Leave INPUT_CONF unset and point at a file that never appears.
	t.Setenv(inputConfigPathEnv, "")
	configDir := t.TempDir()
	path := filepath.Join(configDir, "envoy.yaml")

	// The wait returns at once.
	done := make(chan struct{})
	go func() {
		waitForInputConfig(path, configDir)
		close(done)
	}()
	g.Eventually(done).WithTimeout(5*time.Second).Should(BeClosed(), "without INPUT_CONF the default path should not be waited for")
}

func TestWaitForInputConfigOutsideConfigDir(t *testing.T) {
	g := NewWithT(t)

	// Set INPUT_CONF to a file that never appears, outside the directory the generated
	// ConfigMap is mounted at.
	path := filepath.Join(t.TempDir(), "envoy.yaml")
	t.Setenv(inputConfigPathEnv, path)

	// The wait returns at once, so reading the file reports that it is missing.
	done := make(chan struct{})
	go func() {
		waitForInputConfig(path, t.TempDir())
		close(done)
	}()
	g.Eventually(done).WithTimeout(5*time.Second).Should(BeClosed(), "a file outside the generated ConfigMap's mount should not be waited for")
}

func TestWaitForInputConfigWithEnv(t *testing.T) {
	g := NewWithT(t)

	// Set INPUT_CONF to a file that does not exist yet, in the directory the generated
	// ConfigMap is mounted at.
	configDir := t.TempDir()
	path := filepath.Join(configDir, "envoy-abc.yaml")
	t.Setenv(inputConfigPathEnv, path)

	// Start waiting, and check the wait has not returned.
	done := make(chan struct{})
	go func() {
		waitForInputConfig(path, configDir)
		close(done)
	}()
	g.Consistently(done).WithTimeout(1500*time.Millisecond).ShouldNot(BeClosed(), "the wait should last while the file is missing")

	// Write the file; the wait returns.
	g.Expect(os.WriteFile(path, []byte("node: {}"), 0o600)).To(Succeed())
	g.Eventually(done).WithTimeout(5*time.Second).Should(BeClosed(), "the wait should return once the file appears")
}

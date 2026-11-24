package envoyinit

import (
	"testing"

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

package envoyinit

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
)

// addSystemCASecret appends a static secret named SYSTEM_CA_CERT, which trusts the CA
// bundle at caPath, to the bootstrap's static_resources.secrets, and returns the
// bootstrap as YAML.
//
// It edits the bootstrap as a generic document rather than an Envoy Bootstrap proto, so
// a field or extension type this binary does not know reaches Envoy with its values, and
// Envoy alone decides whether the bootstrap is valid. Re-encoding can change key order
// and number formatting, but not values: integers are not rounded through float64. A
// bootstrap may spell the field static_resources or staticResources; the secret goes
// into whichever is present, and into a new static_resources when neither is.
func addSystemCASecret(bootstrapYAML []byte, caPath string) ([]byte, error) {
	// Decode the bootstrap into a generic map without rounding integers.
	bootstrap, err := utils.DecodeYAMLMap(bootstrapYAML)
	if err != nil {
		return nil, fmt.Errorf("the bootstrap %w", err)
	}

	// Find static_resources under either spelling, creating it when neither is set.
	key := "static_resources"
	if _, ok := bootstrap[key]; !ok {
		if _, ok := bootstrap["staticResources"]; ok {
			key = "staticResources"
		}
	}
	staticResources, ok := bootstrap[key].(map[string]any)
	if !ok {
		if bootstrap[key] != nil {
			return nil, fmt.Errorf("the bootstrap's %s is not a mapping", key)
		}
		staticResources = map[string]any{}
		bootstrap[key] = staticResources
	}

	// Append the secret to the existing secrets.
	secrets, ok := staticResources["secrets"].([]any)
	if !ok && staticResources["secrets"] != nil {
		return nil, fmt.Errorf("the bootstrap's %s.secrets is not a list", key)
	}
	staticResources["secrets"] = append(secrets, map[string]any{
		"name": utils.SystemCaSecretName,
		"validation_context": map[string]any{
			"trusted_ca": map[string]any{
				"filename": caPath,
			},
		},
	})

	// Encode the result as YAML.
	out, err := yaml.Marshal(bootstrap)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the bootstrap: %w", err)
	}
	return out, nil
}

// Package bootstrappatch merges user overlays from GatewayParameters into the Envoy
// bootstrap that the deployer generates for a managed gateway proxy.
package bootstrappatch

import (
	"bytes"
	"errors"
	"fmt"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"
)

// templateDelim opens a Go text/template action. envoyinit runs text/template over the
// whole bootstrap when the proxy pod starts, so an overlay containing it would be
// changed, or fail to parse, in the pod, where the controller cannot see it.
const templateDelim = "{{"

var errTemplateDelim = errors.New(`contains "{{", which the proxy treats as the start of a template when it starts; remove it`)

// outputMarshaler writes snake_case field names, matching the generated bootstrap and
// Envoy's documentation, so users can copy entries from the ConfigMap into an overlay.
var outputMarshaler = protojson.MarshalOptions{UseProtoNames: true}

// ParseBootstrap parses a complete bootstrap written as YAML or JSON.
func ParseBootstrap(data []byte) (*envoybootstrapv3.Bootstrap, error) {
	// Convert to JSON, then let protojson resolve field names and `@type`s.
	jsn, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, err
	}
	bs := &envoybootstrapv3.Bootstrap{}
	if err := protojson.Unmarshal(jsn, bs); err != nil {
		return nil, err
	}
	return bs, nil
}

// parseOverlay parses a partial bootstrap written as JSON, as stored in
// GatewayParameters. It rejects unknown fields, `@type`s the controller does not link,
// and any template delimiter.
func parseOverlay(raw []byte) (*envoybootstrapv3.Bootstrap, error) {
	// Parse strictly, so a misspelled field fails here rather than being dropped.
	ov := &envoybootstrapv3.Bootstrap{}
	if err := protojson.Unmarshal(raw, ov); err != nil {
		return nil, fmt.Errorf("does not parse as an Envoy Bootstrap: %w", err)
	}

	// Look for the template delimiter in the re-marshalled overlay rather than in raw, so
	// JSON escapes such as { cannot hide it from the check.
	normalized, err := protojson.Marshal(ov)
	if err != nil {
		return nil, fmt.Errorf("does not marshal as an Envoy Bootstrap: %w", err)
	}
	if bytes.Contains(normalized, []byte(templateDelim)) {
		return nil, errTemplateDelim
	}
	return ov, nil
}

// Marshal renders a bootstrap as YAML with sorted keys and snake_case field names.
// protojson varies its whitespace between builds on purpose; converting to YAML removes
// it, so identical content gives identical bytes, and the same pod-template checksum,
// across controller versions.
func Marshal(bs *envoybootstrapv3.Bootstrap) ([]byte, error) {
	jsn, err := outputMarshaler.Marshal(bs)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jsn)
}

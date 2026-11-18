package bootstrappatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// ValidationKey is the key in a Gateway's generated bootstrap ConfigMap that holds
	// the translator's inputs for validating the Gateway's xDS: the validation subset of
	// its bootstrap and the ports the bootstrap listens on. Envoy does not read it.
	ValidationKey = "xds_validation.json"
	// ValidationAnnotation is the pod-template annotation that holds the validation
	// subset of the bootstrap the pod starts with, which the translator uses to validate
	// xDS for that pod.
	ValidationAnnotation = "kgateway.dev/xds-validation-bootstrap"
	// MaxValidationSubsetSize is the largest encoded validation subset the deployer
	// writes. It keeps the pod annotation well under the 256 KiB limit on all of a
	// pod's annotations.
	MaxValidationSubsetSize = 64 * 1024
	// EmptyValidationSubset is the encoded subset of a bootstrap without static runtime
	// layers. Envoy validates under it as it does under its defaults.
	EmptyValidationSubset = "{}"
)

// ValidationData is the content of the ValidationKey.
type ValidationData struct {
	// Bootstrap is the validation subset, as EncodeValidationSubset writes it.
	Bootstrap json.RawMessage `json:"bootstrap"`
	// ReservedPorts are the ports the bootstrap listens on, sorted. They replace the
	// static reserved ports for the Gateway.
	ReservedPorts []uint32 `json:"reservedPorts"`
}

// ValidationSubset returns the fields of bs that change which xDS configuration Envoy
// accepts and that Envoy can load in the controller: the static runtime layers, which
// set limits such as re2.max_program_size.error_level. Admin, RTDS and disk layers are
// left out, since their values are not in the bootstrap. default_regex_engine is left
// out because its only engine, GoogleRE2, has no settings, so it accepts what Envoy's
// default engine accepts. Add a field here only with a test that shows it changes
// what Envoy accepts, as TestGatewayValidatorWithEnvoy in
// pkg/kgateway/translator/irtranslator does for runtime layers.
func ValidationSubset(bs *envoybootstrapv3.Bootstrap) *envoybootstrapv3.Bootstrap {
	// Keep each static layer, in order, since a later layer overrides an earlier one.
	out := &envoybootstrapv3.Bootstrap{}
	for _, l := range bs.GetLayeredRuntime().GetLayers() {
		if l.GetStaticLayer() == nil {
			continue
		}
		if out.LayeredRuntime == nil {
			out.LayeredRuntime = &envoybootstrapv3.LayeredRuntime{}
		}
		out.LayeredRuntime.Layers = append(out.LayeredRuntime.Layers, l)
	}
	return out
}

// EncodeValidationSubset encodes subset as compact proto JSON with snake_case field
// names and sorted keys. Equal subsets always encode to the same string, so the
// translator can compare subsets as strings.
func EncodeValidationSubset(subset *envoybootstrapv3.Bootstrap) (string, error) {
	// protojson varies its whitespace between builds on purpose, so decode its output
	// and encode it again with encoding/json, which sorts keys and writes no
	// whitespace. UseNumber keeps each number as protojson wrote it.
	jsn, err := outputMarshaler.Marshal(subset)
	if err != nil {
		return "", err
	}
	dec := json.NewDecoder(bytes.NewReader(jsn))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ParseValidationSubset parses a subset that EncodeValidationSubset wrote.
func ParseValidationSubset(s string) (*envoybootstrapv3.Bootstrap, error) {
	bs := &envoybootstrapv3.Bootstrap{}
	if err := protojson.Unmarshal([]byte(s), bs); err != nil {
		return nil, err
	}
	return bs, nil
}

// EncodeValidationData encodes the content of the ValidationKey: subset, as
// EncodeValidationSubset wrote it, and the reserved ports.
func EncodeValidationData(subset string, reservedPorts []uint32) (string, error) {
	ports := slices.Sorted(slices.Values(reservedPorts))
	out, err := json.Marshal(ValidationData{Bootstrap: json.RawMessage(subset), ReservedPorts: slices.Compact(ports)})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// DecodeValidationData decodes the content of the ValidationKey.
func DecodeValidationData(s string) (ValidationData, error) {
	var out ValidationData
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return ValidationData{}, fmt.Errorf("%s does not decode: %w", ValidationKey, err)
	}
	return out, nil
}

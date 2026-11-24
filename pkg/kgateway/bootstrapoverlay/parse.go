// Package bootstrapoverlay merges user overlays from GatewayParameters into the Envoy
// bootstrap that the deployer generates for a managed gateway proxy.
package bootstrapoverlay

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"

	// protojson resolves each `@type` in a bootstrap through Go's global type registry,
	// so every type a bootstrap or an overlay uses must be linked into the binary that
	// parses it. This links every go-control-plane type, and the cncf/xds TypedStruct
	// wrappers, so the parse never rejects a type the proxy's Envoy supports. A type the
	// proxy's Envoy is not built with, such as a contrib extension, parses here and is
	// rejected by Envoy's validation. TestGeneratedBootstrapParses in
	// pkg/kgateway/deployer checks that these are linked.
	_ "github.com/kgateway-dev/kgateway/v2/pkg/utils/filter_types"
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

// ListenerPorts returns, sorted and without duplicates, the TCP ports the bootstrap
// listens on: the admin address, and each static listener's address and additional
// addresses. A pipe or internal address has no port and is skipped. A UDP address is
// skipped too: Gateway listeners and the Service's ports are TCP, and Envoy binds a
// TCP and a UDP socket on the same port number. A listener with bind_to_port set to
// false opens no socket and is included all the same: Envoy rejects a second listener
// on the address of a listener that does not bind.
func ListenerPorts(bs *envoybootstrapv3.Bootstrap) []uint32 {
	// Collect the addresses.
	addrs := []*envoycorev3.Address{bs.GetAdmin().GetAddress()}
	for _, l := range bs.GetStaticResources().GetListeners() {
		addrs = append(addrs, l.GetAddress())
		for _, a := range l.GetAdditionalAddresses() {
			addrs = append(addrs, a.GetAddress())
		}
	}

	// Keep each TCP socket address's port.
	var ports []uint32
	for _, a := range addrs {
		sa := a.GetSocketAddress()
		if sa.GetProtocol() == envoycorev3.SocketAddress_UDP {
			continue
		}
		if port := sa.GetPortValue(); port != 0 {
			ports = append(ports, port)
		}
	}
	slices.Sort(ports)
	return slices.Compact(ports)
}

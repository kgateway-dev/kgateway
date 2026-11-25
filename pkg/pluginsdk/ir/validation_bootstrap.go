package ir

import (
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
)

// GatewayValidationBootstraps holds the parts of a Gateway's current proxy bootstrap
// that change which xDS configuration Envoy accepts: the bootstrap new pods start
// with. A nil *GatewayValidationBootstraps means the proxy runs with Envoy's defaults
// and listens only on the static reserved ports.
type GatewayValidationBootstraps struct {
	Subset ValidationBootstrap
	// ReservedPorts are the ports the current bootstrap listens on. They are reserved
	// in addition to the static reserved ports.
	ReservedPorts []uint32
}

// ValidationBootstrap is the validation subset of one bootstrap.
type ValidationBootstrap struct {
	// Subset is the encoded subset; empty when it sets nothing, so Envoy's defaults apply.
	Subset string
	// Bootstrap is Subset, parsed.
	// +noKrtEquals derived from Subset
	Bootstrap *envoybootstrapv3.Bootstrap `json:"-"`
}

// PodBootstrap is what a proxy pod's validation annotation says about the bootstrap
// the pod started with: the parts that decide which xDS configuration it accepts.
type PodBootstrap struct {
	// Subset is the encoded validation subset, compared with ValidationBootstrap.Subset;
	// empty when it sets nothing, so Envoy's defaults apply.
	Subset string
	// Ports are the ports the bootstrap listens on.
	Ports []uint32
}

// Equals compares the subset and the ports.
func (b *PodBootstrap) Equals(in *PodBootstrap) bool {
	if b == nil || in == nil {
		return b == in
	}
	return b.Subset == in.Subset && slices.Equal(b.Ports, in.Ports)
}

// GetReservedPorts returns ReservedPorts, or nil when g is nil.
func (g *GatewayValidationBootstraps) GetReservedPorts() []uint32 {
	if g == nil {
		return nil
	}
	return g.ReservedPorts
}

// GetSubset returns Subset, or the zero value, Envoy's defaults, when g is nil.
func (g *GatewayValidationBootstraps) GetSubset() ValidationBootstrap {
	if g == nil {
		return ValidationBootstrap{}
	}
	return g.Subset
}

// Equals compares the encoded subset and the reserved ports.
func (g *GatewayValidationBootstraps) Equals(in *GatewayValidationBootstraps) bool {
	if g == nil || in == nil {
		return g == in
	}
	return g.Subset.Subset == in.Subset.Subset && slices.Equal(g.ReservedPorts, in.ReservedPorts)
}

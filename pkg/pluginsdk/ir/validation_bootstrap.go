package ir

import (
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
)

// GatewayValidationBootstraps holds the parts of a Gateway's proxy bootstraps that
// change which xDS configuration Envoy accepts. Every proxy of the Gateway receives
// the same xDS configuration, including pods still running an earlier bootstrap
// during a rollout, so the configuration must pass validation under each of them.
// A nil *GatewayValidationBootstraps means every proxy runs with Envoy's defaults
// and listens only on the static reserved ports.
type GatewayValidationBootstraps struct {
	// Subsets are the distinct validation subsets of the Gateway's bootstraps:
	// first the current one, then one for each other subset a running pod carries.
	Subsets []ValidationBootstrap
	// ReservedPorts are the ports the Gateway's current bootstrap listens on, which
	// replace the static reserved ports. Empty means the static reserved ports.
	ReservedPorts []uint32
}

// ValidationBootstrap is the validation subset of one bootstrap.
type ValidationBootstrap struct {
	// Subset is the encoded subset; empty when it sets nothing, so Envoy's defaults
	// apply.
	Subset string
	// Pod names a running pod that carries this subset. It is empty for the current
	// subset.
	Pod string
	// Bootstrap is Subset, parsed.
	// +noKrtEquals derived from Subset
	Bootstrap *envoybootstrapv3.Bootstrap `json:"-"`
}

// GetReservedPorts returns ReservedPorts, or nil when g is nil.
func (g *GatewayValidationBootstraps) GetReservedPorts() []uint32 {
	if g == nil {
		return nil
	}
	return g.ReservedPorts
}

// GetSubsets returns Subsets, or nil when g is nil.
func (g *GatewayValidationBootstraps) GetSubsets() []ValidationBootstrap {
	if g == nil {
		return nil
	}
	return g.Subsets
}

// Equals compares the encoded subsets, the pods named for them and the reserved ports.
func (g *GatewayValidationBootstraps) Equals(in *GatewayValidationBootstraps) bool {
	if g == nil || in == nil {
		return g == in
	}
	return slices.EqualFunc(g.Subsets, in.Subsets, func(a, b ValidationBootstrap) bool {
		return a.Subset == b.Subset && a.Pod == b.Pod
	}) && slices.Equal(g.ReservedPorts, in.ReservedPorts)
}

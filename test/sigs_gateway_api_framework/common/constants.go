//go:build e2e

package common

// GatewayName is the name of the shared Gateway resource that every
// conformance feature test attaches its HTTPRoute to. It matches the
// metadata.name of the Gateway in the base manifests applied by the suite.
const GatewayName = "basic-gateway"

package annotations

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

// PerConnectionBufferLimit is the annotation key for the per connection buffer limit.
// It is used to set the per connection buffer limit for the gateway.
// The value is a string representing the limit, e.g "64Ki".
// The limit is applied to all listeners in the gateway.
const PerConnectionBufferLimit = "kgateway.dev/per-connection-buffer-limit"

// AlpnProtocols is the annotation key used to set the ALPN protocols for a TLS listener.
// The value is a comma separated list of protocols, e.g "h2,http/1.1".
// If not present, the listener will use the default ALPN protocols ("h2", "http/1.1").
// Use in the TLS options field of a TLS listener.
// example:
// ```
// tls:
//
//	options:
//	  kgateway.dev/alpn-protocols: "h2,http/1.1"
//
// ```
const AlpnProtocols gwv1.AnnotationKey = "kgateway.dev/alpn-protocols"

// AllowEmptyAlpnProtocols is an annotation value for the ALPN protocols.
// It is used to allow empty ALPN protocols for a TLS listener.
// The value is a boolean, e.g "true".
const AllowEmptyAlpnProtocols gwv1.AnnotationValue = "allow-empty"

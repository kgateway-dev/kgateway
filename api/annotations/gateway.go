package annotations

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

const (
	// PerConnectionBufferLimit is the annotation key for the per connection buffer limit.
	// It is used to set the per connection buffer limit for the gateway.
	// The value is a string representing the limit, e.g "64Ki".
	// The limit is applied to all listeners in the gateway.
	//
	// Deprecated: This annotation is deprecated. Use ListenerPolicy with perConnectionBufferLimitBytes instead.
	// This annotation will be removed in v2.3.
	PerConnectionBufferLimit gwv1.AnnotationKey = "kgateway.dev/per-connection-buffer-limit"

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
	AlpnProtocols gwv1.AnnotationKey = "kgateway.dev/alpn-protocols"

	// AllowEmptyAlpnProtocols is an annotation value for the ALPN protocols.
	// It is used to allow empty ALPN protocols for a TLS listener.
	// The value is a boolean, e.g "true".
	AllowEmptyAlpnProtocols gwv1.AnnotationValue = "allow-empty"

	// CipherSuites is the annotation key used to set the cipher suites for a TLS listener.
	// The value is a comma separated list of cipher suites, e.g "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384".
	// Use in the TLS options field of a TLS listener.
	CipherSuites gwv1.AnnotationKey = "kgateway.dev/cipher-suites"

	// EcdhCurves is the annotation key used to set the ECDH curves for a TLS listener.
	// The value is a comma separated list of curves, e.g "X25519MLKEM768,X25519,P-256".
	// Use in the TLS options field of a TLS listener.
	EcdhCurves gwv1.AnnotationKey = "kgateway.dev/ecdh-curves"

	// MinTLSVersion is the annotation key used to set the minimum TLS version for a TLS listener.
	// The value is a string representing the version, e.g "1.2".
	// Use in the TLS options field of a TLS listener.
	MinTLSVersion gwv1.AnnotationKey = "kgateway.dev/min-tls-version"

	// MaxTLSVersion is the annotation key used to set the maximum TLS version for a TLS listener.
	// The value is a string representing the version, e.g "1.3".
	// Use in the TLS options field of a TLS listener.
	MaxTLSVersion gwv1.AnnotationKey = "kgateway.dev/max-tls-version"

	// VerifySubjectAltNames is the annotation key used to set the verify subject alt names for a TLS listener.
	// The value is a comma separated list of subject alt names, e.g "example.com,www.example.com".
	// Use in the TLS options field of a TLS listener.
	// Note: This annotation requires a trusted CA to be configured
	VerifySubjectAltNames gwv1.AnnotationKey = "kgateway.dev/verify-subject-alt-names"

	// VerifyCertificateHash is the annotation key used to set the verify certificate hash used by the client.
	// The value is a comma or "-" separated list of certificate hashes which may be whitespace padded for readability.
	// Valid values are sha256 hashes in hex format, e.g "7D86C6654C8229364ECFE4D4964C69410090AE09E9B4D0C9B2AD7854175AD51D" or "7D:86:C6:65:4C:82:29:36:4E:CF:E4:D4:96:4C:69:41:00:90:AE:09:E9:B4:D0:C9:B2:AD:78:54:17:5A:D5:1D".
	// All characters, including formatting, are limited to 4096 characters by the annotation value specification https://gateway-api.sigs.k8s.io/reference/1.4/spec/#annotationvalue
	VerifyCertificateHash gwv1.AnnotationKey = "kgateway.dev/verify-certificate-hash"
)

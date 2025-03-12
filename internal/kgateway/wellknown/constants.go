package wellknown

const (
	// Env variable that indicates the Istio sidecar injection is enabled via istioIntegration.enableIstioSidecarOnGateway
	// on the helm chart. If enabled, the gateway proxy is assumed to have an istio sidecar injected.
	IstioInjectionEnabled = "ENABLE_ISTIO_SIDECAR_ON_GATEWAY"

	// Note: These are coming from istio: https://github.com/istio/istio/blob/fa321ebd2a1186325788b0f461aa9f36a1a8d90e/pilot/pkg/model/service.go#L206
	// IstioCertSecret is the secret that holds the server cert and key for Istio mTLS
	IstioCertSecret = "istio_server_cert"

	// IstioValidationContext is the secret that holds the root cert for Istio mTLS
	IstioValidationContext = "istio_validation_context"

	// IstioTlsModeLabel is the Istio injection label added to workloads in mesh
	IstioTlsModeLabel = "security.istio.io/tlsMode"

	// IstioMutualTLSModeLabel implies that the endpoint is ready to receive Istio mTLS connections.
	IstioMutualTLSModeLabel = "istio"

	// TLSModeLabelShortname name used for determining endpoint level tls transport socket configuration
	TLSModeLabelShortname = "tlsMode"
)

const (
	SdsClusterName = "gateway_proxy_sds"
	SdsTargetURI   = "127.0.0.1:8234"
)

const (
	InfPoolBackendTransformationFilterName = "inferencepool.backend.transformation.kgateway.io"
	AIBackendTransformationFilterName      = "ai.backend.transformation.kgateway.io"
	AIPolicyTransformationFilterName       = "ai.policy.transformation.kgateway.io"
	AIExtProcFilterName                    = "ai.extproc.kgateway.io"
	SetMetadataFilterName                  = "envoy.filters.http.set_filter_state"
)

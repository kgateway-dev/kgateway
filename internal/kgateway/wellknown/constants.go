package wellknown

const (
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

	// IngressUseWaypointLabel is a Service/ServiceEntry label to ask the ingress to use
	// a waypoint for ingress traffic.
	IngressUseWaypointLabel = "istio.io/ingress-use-waypoint"
)

const (
	SdsClusterName = "gateway_proxy_sds"
	SdsTargetURI   = "127.0.0.1:8234"
)

const (
	InfPoolTransformationFilterName   = "inferencepool.backend.transformation.kgateway.io"
	AIBackendTransformationFilterName = "ai.backend.transformation.kgateway.io"
	AIPolicyTransformationFilterName  = "ai.policy.transformation.kgateway.io"
	AIExtProcFilterName               = "ai.extproc.kgateway.io"
	SetMetadataFilterName             = "envoy.filters.http.set_filter_state"
	ExtprocFilterName                 = "envoy.filters.http.ext_proc"
)

const (
	EnvoyConfigNameMaxLen = 253
)

// AWS constants for lambda and bedrock configuration
const (
	// AccessKey is the key name for in the secret data for the access key id.
	AccessKey = "accessKey"
	// SessionToken is the key name for in the secret data for the session token.
	SessionToken = "sessionToken"
	// SecretKey is the key name for in the secret data for the secret access key.
	SecretKey = "secretKey"
	// DefaultAWSRegion is the default AWS region.
	DefaultAWSRegion = "us-east-1"
)

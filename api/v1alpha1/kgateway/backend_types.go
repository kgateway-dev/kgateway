package kgateway

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=".spec.type",description="Which backend type?"
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.conditions[?(@.type=='Accepted')].status",description="Backend configuration acceptance status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp",description="The age of the backend."

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type Backend struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec BackendSpec `json:"spec"`
	// +optional
	Status BackendStatus `json:"status,omitempty"`
}

// BackendType indicates the type of the backend.
type BackendType string

const (
	// BackendTypeAWS is the type for AWS backends.
	BackendTypeAWS BackendType = "AWS"
	// BackendTypeStatic is the type for static backends.
	BackendTypeStatic BackendType = "Static"
	// BackendTypeDynamicForwardProxy is the type for dynamic forward proxy backends.
	BackendTypeDynamicForwardProxy BackendType = "DynamicForwardProxy"
	// BackendTypeGCP is the type for GCP backends.
	BackendTypeGCP BackendType = "GCP"
)

// BackendSpec defines the desired state of Backend.
// +kubebuilder:validation:XValidation:message="aws backend must be specified when type is 'AWS'",rule="self.type == 'AWS' ? has(self.aws) : true"
// +kubebuilder:validation:XValidation:message="static backend must be specified when type is 'Static'",rule="self.type == 'Static' ? has(self.static) : true"
// +kubebuilder:validation:XValidation:message="dynamicForwardProxy backend must be specified when type is 'DynamicForwardProxy'",rule="self.type == 'DynamicForwardProxy' ? has(self.dynamicForwardProxy) : true"
// +kubebuilder:validation:XValidation:message="gcp backend must be specified when type is 'GCP'",rule="self.type == 'GCP' ? has(self.gcp) : true"
// +kubebuilder:validation:ExactlyOneOf=aws;static;dynamicForwardProxy;gcp
type BackendSpec struct {
	// Type indicates the type of the backend to be used.
	// +kubebuilder:validation:Enum=AWS;Static;DynamicForwardProxy;GCP
	// Deprecated: The Type field is deprecated and will be removed in a future release.
	// The backend type is inferred from the configuration.
	// +optional
	Type *BackendType `json:"type,omitempty"`
	// Aws is the AWS backend configuration.
	// +optional
	Aws *AwsBackend `json:"aws,omitempty"`
	// Static is the static backend configuration.
	// +optional
	Static *StaticBackend `json:"static,omitempty"`
	// DynamicForwardProxy is the dynamic forward proxy backend configuration.
	// +optional
	DynamicForwardProxy *DynamicForwardProxyBackend `json:"dynamicForwardProxy,omitempty"`
	// Gcp is the GCP backend configuration.
	// +optional
	Gcp *GcpBackend `json:"gcp,omitempty"`
}

// AppProtocol defines the application protocol to use when communicating with the backend.
// +kubebuilder:validation:Enum=http2;grpc;grpc-web;kubernetes.io/h2c;kubernetes.io/ws
type AppProtocol string

const (
	// AppProtocolHttp2 is the http2 app protocol.
	AppProtocolHttp2 AppProtocol = "http2"
	// AppProtocolGrpc is the grpc app protocol.
	AppProtocolGrpc AppProtocol = "grpc"
	// AppProtocolGrpcWeb is the grpc-web app protocol.
	AppProtocolGrpcWeb AppProtocol = "grpc-web"
	// AppProtocolKubernetesH2C is the kubernetes.io/h2c app protocol.
	AppProtocolKubernetesH2C AppProtocol = "kubernetes.io/h2c"
	// AppProtocolKubernetesWs is the kubernetes.io/ws app protocol.
	AppProtocolKubernetesWs AppProtocol = "kubernetes.io/ws"
)

// DynamicForwardProxyBackend is the dynamic forward proxy backend configuration.
type DynamicForwardProxyBackend struct {
	// EnableTls enables TLS. When true, the backend will be configured to use TLS. System CA will be used for validation.
	// The hostname will be used for SNI and auto SAN validation.
	// +optional
	EnableTls *bool `json:"enableTls,omitempty"`
}

// AwsBackend is the AWS backend configuration.
// +kubebuilder:validation:ExactlyOneOf=lambda;ec2
// +kubebuilder:validation:XValidation:message="accountId must be specified on aws or aws.lambda for lambda backends",rule="!has(self.lambda) || has(self.accountId) || has(self.lambda.accountId)"
type AwsBackend struct {
	// Lambda configures the AWS Lambda service.
	// +optional
	Lambda *AwsLambda `json:"lambda,omitempty"`

	// Ec2 configures dynamic discovery of AWS EC2 instances.
	// +optional
	Ec2 *AwsEc2 `json:"ec2,omitempty"`

	// AccountId is the AWS account ID to use for the backend.
	// Deprecated: Set accountId on spec.aws.lambda instead. This field is kept for backward compatibility.
	// When both fields are set, spec.aws.lambda.accountId takes precedence.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=12
	// +kubebuilder:validation:Pattern="^[0-9]{12}$"
	AccountId string `json:"accountId,omitempty"`

	// Auth specifies an explicit AWS authentication method for the backend.
	// When omitted, the following credential providers are tried in order, stopping when one
	// of them returns an access key ID and a secret access key (the session token is optional):
	// 1. Environment variables: when the environment variables AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_SESSION_TOKEN are set.
	// 2. AssumeRoleWithWebIdentity API call: when the environment variables AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN are set.
	// 3. EKS Pod Identity: when the environment variable AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE is set.
	//
	// See the Envoy docs for more info:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/aws_request_signing_filter#credentials
	//
	// +optional
	Auth *AwsAuth `json:"auth,omitempty"`

	// Region is the AWS region to use for the backend.
	// Defaults to us-east-1 if not specified.
	// +optional
	// +kubebuilder:default=us-east-1
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern="^[a-z0-9-]+$"
	Region string `json:"region,omitempty"`
}

// AwsAuthType specifies the authentication method to use for the backend.
type AwsAuthType string

const (
	// AwsAuthTypeSecret uses credentials stored in a Kubernetes Secret.
	AwsAuthTypeSecret AwsAuthType = "Secret"
	// AwsAuthTypeAssumeRole assumes an IAM role via STS, chaining off the
	// backend's ambient credentials (the gateway ServiceAccount's IRSA identity
	// for Lambda request signing, or the controller's identity for EC2
	// discovery). The temporary credentials returned by STS are used to
	// interact with the backend.
	AwsAuthTypeAssumeRole AwsAuthType = "AssumeRole"
)

// AwsAuth specifies the authentication method to use for the backend.
// +kubebuilder:validation:XValidation:message="secretRef must be nil if the type is not 'Secret'",rule="!(has(self.secretRef) && self.type != 'Secret')"
// +kubebuilder:validation:XValidation:message="secretRef must be specified when type is 'Secret'",rule="!(!has(self.secretRef) && self.type == 'Secret')"
// +kubebuilder:validation:XValidation:message="assumeRole must be nil if the type is not 'AssumeRole'",rule="!(has(self.assumeRole) && self.type != 'AssumeRole')"
// +kubebuilder:validation:XValidation:message="assumeRole must be specified when type is 'AssumeRole'",rule="!(!has(self.assumeRole) && self.type == 'AssumeRole')"
type AwsAuth struct {
	// Type specifies the authentication method to use for the backend.
	// +required
	// +kubebuilder:validation:Enum=Secret;AssumeRole
	Type AwsAuthType `json:"type"`
	// SecretRef references a Kubernetes Secret containing the AWS credentials.
	// The Secret must have keys "accessKey", "secretKey", and optionally "sessionToken".
	// Required when type is 'Secret'.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
	// AssumeRole configures STS role chaining. The backend's ambient credentials
	// (the gateway ServiceAccount's IRSA identity for Lambda request signing, or the
	// controller's identity for EC2 discovery; more generally any credential resolved
	// by the default provider chain) are used to assume the target role. The resulting
	// temporary credentials are then used to sign requests to the backend (Lambda) or
	// to list instances (EC2). This enables per-backend, least-privilege roles without
	// granting the gateway/controller role direct access to every target.
	// Required when type is 'AssumeRole'.
	// +optional
	AssumeRole *AwsAssumeRole `json:"assumeRole,omitempty"`
}

// AwsAssumeRole configures assuming an IAM role via STS to obtain the credentials
// used to interact with the backend (signing Lambda requests, or listing EC2 instances).
type AwsAssumeRole struct {
	// RoleArn is the ARN of the IAM role to assume, e.g.
	// "arn:aws:iam::123456789012:role/my-invoke-role".
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern="^arn:aws[a-z-]*:iam::[0-9]{12}:role/.+$"
	RoleArn string `json:"roleArn"`
}

const (
	// AwsLambdaInvocationModeSynchronous is the synchronous invocation mode for the lambda function.
	AwsLambdaInvocationModeSynchronous = "Sync"
	// AwsLambdaInvocationModeAsynchronous is the asynchronous invocation mode for the lambda function.
	AwsLambdaInvocationModeAsynchronous = "Async"
)

// AwsLambda configures the AWS Lambda service.
type AwsLambda struct {
	// AccountId is the AWS account ID to use for the backend.
	// This is the preferred location for Lambda backends.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=12
	// +kubebuilder:validation:Pattern="^[0-9]{12}$"
	AccountId string `json:"accountId,omitempty"`

	// EndpointURL is the URL or domain for the Lambda service. This is primarily
	// useful for testing and development purposes. When omitted, the default
	// lambda hostname will be used.
	// +optional
	// +kubebuilder:validation:Pattern="^https?://[-a-zA-Z0-9@:%.+~#?&/=]+$"
	// +kubebuilder:validation:MaxLength=2048
	EndpointURL *string `json:"endpointURL,omitempty"`
	// FunctionName is the name of the Lambda function to invoke.
	// +required
	// +kubebuilder:validation:Pattern="^[A-Za-z0-9-_]{1,140}$"
	FunctionName string `json:"functionName"`
	// InvocationMode defines how to invoke the Lambda function.
	// Defaults to Sync.
	// +optional
	// +kubebuilder:validation:Enum=Sync;Async
	// +kubebuilder:default=Sync
	InvocationMode string `json:"invocationMode,omitempty"`
	// Qualifier is the alias or version for the Lambda function.
	// Valid values include a numeric version (e.g. "1"), an alias name
	// (alphanumeric plus "-" or "_"), or the special literal "$LATEST".
	// +optional
	// +kubebuilder:validation:Pattern="^(\\$LATEST|[0-9]+|[A-Za-z0-9-_]{1,128})$"
	// +kubebuilder:default=$LATEST
	Qualifier string `json:"qualifier,omitempty"`
	// PayloadTransformation specifies payload transformation mode before it is sent to the Lambda function.
	// Defaults to Envoy.
	// +optional
	// +kubebuilder:default=Envoy
	PayloadTransformMode AWSLambdaPayloadTransformMode `json:"payloadTransformMode,omitempty"`
}

// AwsAddressType defines which EC2 IP address to route to.
// +kubebuilder:validation:Enum=PrivateIP;PublicIP
type AwsAddressType string

const (
	// AwsAddressTypePrivateIP routes to the instance private IP.
	AwsAddressTypePrivateIP AwsAddressType = "PrivateIP"
	// AwsAddressTypePublicIP routes to the instance public IP.
	AwsAddressTypePublicIP AwsAddressType = "PublicIP"
)

// AwsEc2 configures dynamic discovery of EC2 instances.
type AwsEc2 struct {
	// Port is the port to use for discovered instances.
	// Defaults to 80.
	// +optional
	// +kubebuilder:default=80
	Port gwv1.PortNumber `json:"port,omitempty"`

	// AddressType selects whether to route to the instance private or public IP.
	// Defaults to PrivateIP.
	// +optional
	// +kubebuilder:default=PrivateIP
	AddressType AwsAddressType `json:"addressType,omitempty"`

	// Filters select which instances should be associated with this backend.
	// When multiple filters are provided, an instance must match all of them.
	// If this list is omitted or empty, all running instances in the configured
	// region are selected. Be careful: an accidentally empty filter list broadens
	// the backend to the whole regional fleet rather than matching nothing.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Filters []AwsTagFilter `json:"filters,omitempty"`
}

// AwsTagFilter matches EC2 instances by tag.
// +kubebuilder:validation:ExactlyOneOf=key;keyValue
type AwsTagFilter struct {
	// Key matches instances that contain the given tag key, regardless of value.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Key *string `json:"key,omitempty"`

	// KeyValue matches instances that contain the given tag key/value pair.
	// +optional
	KeyValue *AwsTagKeyValueFilter `json:"keyValue,omitempty"`
}

// AwsTagKeyValueFilter matches EC2 instances by a tag key/value pair.
type AwsTagKeyValueFilter struct {
	// Key is the tag key to match.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Key string `json:"key"`

	// Value is the tag value to match.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Value string `json:"value"`
}

// AWSLambdaPayloadTransformMode defines the transformation mode for the payload in the request
// before it is sent to the AWS Lambda function.
//
// +kubebuilder:validation:Enum=None;Envoy
type AWSLambdaPayloadTransformMode string

const (
	// AWSLambdaPayloadTransformNone indicates that the payload will not be transformed using Envoy's
	// built-in transformation before it is sent to the Lambda function.
	// Note: Transformation policies configured on the route will still apply.
	AWSLambdaPayloadTransformNone AWSLambdaPayloadTransformMode = "None"

	// AWSLambdaPayloadTransformEnvoy indicates that the payload will be transformed using Envoy's
	// built-in transformation. Refer to
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/aws_lambda_filter#configuration-as-a-listener-filter
	// for more details on how Envoy transforms the payload.
	AWSLambdaPayloadTransformEnvoy AWSLambdaPayloadTransformMode = "Envoy"
)

// StaticBackend references a static list of hosts.
type StaticBackend struct {
	// Hosts is a list of hosts to use for the backend.
	// +required
	// +kubebuilder:validation:MinItems=1
	Hosts []Host `json:"hosts"`

	// AppProtocol is the application protocol to use when communicating with the backend.
	// +optional
	AppProtocol *AppProtocol `json:"appProtocol,omitempty"`
}

// GcpBackend is the GCP backend configuration.
type GcpBackend struct {
	// Host is the hostname of the GCP service to connect to.
	// This will be used for SNI and as the target address.
	// +required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Audience is the GCP service account audience URL.
	// When omitted, defaults to "https://{host}".
	// This is used by the GCP authn filter to request the appropriate token.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Audience *string `json:"audience,omitempty"`
}

// Host defines a static backend host.
type Host struct {
	// Host is the host name to use for the backend.
	// +kubebuilder:validation:MinLength=1
	// +required
	Host string `json:"host"`
	// Port is the port to use for the backend.
	// +required
	Port gwv1.PortNumber `json:"port"`
}

// BackendConditionType is a type of condition for a Backend. This type should be
// used with a Backend resource Status.Conditions field.
type BackendConditionType string

// BackendConditionReason is a reason for a Backend condition.
type BackendConditionReason string

const (
	// BackendConditionAccepted indicates whether the Backend was accepted, or rejected
	// because it failed to translate.
	BackendConditionAccepted BackendConditionType = "Accepted"

	// BackendReasonAccepted is used with Accepted=True when the Backend translated successfully.
	BackendReasonAccepted BackendConditionReason = "Accepted"

	// BackendReasonInvalid is used with Accepted=False when the Backend failed to translate.
	BackendReasonInvalid BackendConditionReason = "Invalid"

	// BackendConditionEndpointsDiscovered indicates whether runtime endpoint discovery
	// (e.g. AWS EC2 instance discovery) succeeded for backends that resolve their
	// endpoints dynamically. It is only set on backends that perform such discovery.
	BackendConditionEndpointsDiscovered BackendConditionType = "EndpointsDiscovered"

	// BackendReasonDiscovered is used with EndpointsDiscovered=True when the last
	// discovery poll succeeded and resolved at least one active endpoint.
	BackendReasonDiscovered BackendConditionReason = "Discovered"

	// BackendReasonNoMatchingInstances is used with EndpointsDiscovered=False when the
	// last discovery poll succeeded but resolved no endpoints (e.g. no instances matched
	// the configured filters).
	BackendReasonNoMatchingInstances BackendConditionReason = "NoMatchingInstances"

	// BackendReasonCredentialError is used with EndpointsDiscovered=False when discovery
	// credentials are missing or cannot be resolved (e.g. an unresolved secret reference
	// or malformed credential data).
	//
	//nolint:gosec // G101: this is a status condition reason, not a credential.
	BackendReasonCredentialError BackendConditionReason = "CredentialError"

	// BackendReasonAuthorizationError is used with EndpointsDiscovered=False when the
	// discovery provider rejected the request for authentication or authorization reasons.
	BackendReasonAuthorizationError BackendConditionReason = "AuthorizationError"

	// BackendReasonDiscoveryError is used with EndpointsDiscovered=False when discovery
	// failed for a transient or otherwise unclassified reason.
	BackendReasonDiscoveryError BackendConditionReason = "DiscoveryError"

	// BackendReasonDegraded is used with EndpointsDiscovered=False when the last discovery
	// poll failed but the backend is still serving endpoints carried forward from a previous
	// successful poll. It distinguishes a degraded-but-serving backend from one that is hard
	// down (which keeps its specific failure reason, e.g. AuthorizationError, with no
	// endpoints) so operators can alert on the two cases differently. The underlying failure
	// cause is preserved in the condition message.
	BackendReasonDegraded BackendConditionReason = "Degraded"
)

// BackendStatus defines the observed state of Backend.
type BackendStatus struct {
	// Conditions is the list of conditions for the backend.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type BackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backend `json:"items"`
}

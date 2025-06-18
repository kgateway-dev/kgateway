package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=".spec.type",description="Which backend type?"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp",description="The age of the backend."

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type Backend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackendSpec   `json:"spec,omitempty"`
	Status BackendStatus `json:"status,omitempty"`
}

// BackendType indicates the type of the backend.
type BackendType string

const (
	// BackendTypeAI is the type for AI backends.
	BackendTypeAI BackendType = "AI"
	// BackendTypeAWS is the type for AWS backends.
	BackendTypeAWS BackendType = "AWS"
	// BackendTypeStatic is the type for static backends.
	BackendTypeStatic BackendType = "Static"
	// BackendTypeDynamicForwardProxy is the type for dynamic forward proxy backends.
	BackendTypeDynamicForwardProxy BackendType = "DynamicForwardProxy"
)

// BackendSpec defines the desired state of Backend.
// +union
// +kubebuilder:validation:XValidation:message="ai backend must be nil if the type is not 'ai'",rule="!(has(self.ai) && self.type != 'AI')"
// +kubebuilder:validation:XValidation:message="ai backend must be specified when type is 'ai'",rule="!(!has(self.ai) && self.type == 'AI')"
// +kubebuilder:validation:XValidation:message="aws backend must be nil if the type is not 'aws'",rule="!(has(self.aws) && self.type != 'AWS')"
// +kubebuilder:validation:XValidation:message="aws backend must be specified when type is 'aws'",rule="!(!has(self.aws) && self.type == 'AWS')"
// +kubebuilder:validation:XValidation:message="static backend must be nil if the type is not 'static'",rule="!(has(self.static) && self.type != 'Static')"
// +kubebuilder:validation:XValidation:message="static backend must be specified when type is 'static'",rule="!(!has(self.static) && self.type == 'Static')"
// +kubebuilder:validation:XValidation:message="dynamic forward proxy backend must be nil if the type is not 'dynamicForwardProxy'",rule="!(has(self.dynamicForwardProxy) && self.type != 'DynamicForwardProxy')"
// +kubebuilder:validation:XValidation:message="dynamic forward proxy backend must be specified when type is 'dynamicForwardProxy'",rule="!(!has(self.dynamicForwardProxy) && self.type == 'DynamicForwardProxy')"
type BackendSpec struct {
	// Type indicates the type of the backend to be used.
	// +unionDiscriminator
	// +kubebuilder:validation:Enum=AI;AWS;Static;DynamicForwardProxy
	// +required
	Type BackendType `json:"type"`
	// AI is the AI backend configuration.
	// +optional
	AI *AIBackend `json:"ai,omitempty"`
	// Aws is the AWS backend configuration.
	// +optional
	Aws *AwsBackend `json:"aws,omitempty"`
	// Static is the static backend configuration.
	// +optional
	Static *StaticBackend `json:"static,omitempty"`
	// DynamicForwardProxy is the dynamic forward proxy backend configuration.
	// +optional
	DynamicForwardProxy *DynamicForwardProxyBackend `json:"dynamicForwardProxy,omitempty"`
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
	EnableTls bool `json:"enableTls,omitempty"`
}

// AwsBackend is the AWS backend configuration.
type AwsBackend struct {
	// AccountId is the AWS account ID to use for the backend.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=12
	// +kubebuilder:validation:Pattern="^[0-9]{12}$"
	AccountId string `json:"accountId"`
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
	// Lambda configures the AWS lambda service.
	// +optional
	Lambda *AwsLambda `json:"lambda,omitempty"`
	// Region is the AWS region to use for the backend.
	// Defaults to us-east-1 if not specified.
	// +optional
	// +kubebuilder:default=us-east-1
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern="^[a-z0-9-]+$"
	Region *string `json:"region,omitempty"`
}

// AwsAuthType specifies the authentication method to use for the backend.
type AwsAuthType string

const (
	// AwsAuthTypeSecret uses credentials stored in a Kubernetes Secret.
	AwsAuthTypeSecret AwsAuthType = "Secret"
)

// AwsAuth specifies the authentication method to use for the backend.
// +union
// +kubebuilder:validation:XValidation:message="secretRef must be nil if the type is not 'Secret'",rule="!(has(self.secretRef) && self.type != 'Secret')"
// +kubebuilder:validation:XValidation:message="secretRef must be specified when type is 'Secret'",rule="!(!has(self.secretRef) && self.type == 'Secret')"
type AwsAuth struct {
	// Type specifies the authentication method to use for the backend.
	// +unionDiscriminator
	// +required
	// +kubebuilder:validation:Enum=Secret
	Type AwsAuthType `json:"type"`
	// SecretRef references a Kubernetes Secret containing the AWS credentials.
	// The Secret must have keys "accessKey", "secretKey", and optionally "sessionToken".
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

const (
	// AwsLambdaInvocationModeSynchronous is the synchronous invocation mode for the lambda function.
	AwsLambdaInvocationModeSynchronous = "Sync"
	// AwsLambdaInvocationModeAsynchronous is the asynchronous invocation mode for the lambda function.
	AwsLambdaInvocationModeAsynchronous = "Async"
)

// AwsLambda configures the AWS lambda service.
type AwsLambda struct {
	// EndpointURL is the URL or domain for the Lambda service. This is primarily
	// useful for testing and development purposes. When omitted, the default
	// lambda hostname will be used.
	// +optional
	// +kubebuilder:validation:Pattern="^https?://[-a-zA-Z0-9@:%.+~#?&/=]+$"
	// +kubebuilder:validation:MaxLength=2048
	EndpointURL string `json:"endpointURL,omitempty"`
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
	Qualifier string `json:"qualifier,omitempty"`
	// PayloadTransformation specifies payload transformation mode before it is sent to the Lambda function.
	// Defaults to Envoy.
	// +optional
	// +kubebuilder:default=Envoy
	PayloadTransformMode AWSLambdaPayloadTransformMode `json:"payloadTransformMode,omitempty"`
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
	Hosts []Host `json:"hosts,omitempty"`

	// AppProtocol is the application protocol to use when communicating with the backend.
	// +optional
	// +kubebuilder:validation:Optional
	AppProtocol *AppProtocol `json:"appProtocol,omitempty"`
}

// Host defines a static backend host.
type Host struct {
	// Host is the host name to use for the backend.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// Port is the port to use for the backend.
	// +required
	Port gwv1.PortNumber `json:"port"`
	// InsecureSkipVerify allows skipping ssl validation for custom hosts
	// +optional
	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`
}

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

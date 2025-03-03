package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backends/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway,shortName=be
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
	BackendTypeAI BackendType = "ai"
	// BackendTypeAWS is the type for AWS backends.
	BackendTypeAWS BackendType = "aws"
	// BackendTypeStatic is the type for static backends.
	BackendTypeStatic BackendType = "static"
)

// BackendSpec defines the desired state of Backend.
// +union
// +kubebuilder:validation:XValidation:message="ai backend must be nil if the type is not 'ai'",rule="!(has(self.ai) && self.type != 'ai')"
// +kubebuilder:validation:XValidation:message="ai backend must be specified when type is 'ai'",rule="!(!has(self.ai) && self.type == 'ai')"
// +kubebuilder:validation:XValidation:message="aws backend must be nil if the type is not 'aws'",rule="!(has(self.aws) && self.type != 'aws')"
// +kubebuilder:validation:XValidation:message="aws backend must be specified when type is 'aws'",rule="!(!has(self.aws) && self.type == 'aws')"
// +kubebuilder:validation:XValidation:message="static backend must be nil if the type is not 'static'",rule="!(has(self.static) && self.type != 'static')"
// +kubebuilder:validation:XValidation:message="static backend must be specified when type is 'static'",rule="!(!has(self.static) && self.type == 'static')"
type BackendSpec struct {
	// Type indicates the type of the backend to be used.
	// +unionDiscriminator
	// +kubebuilder:validation:Enum=ai;aws;static
	// +kubebuilder:validation:Required
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
}

// AwsBackend is the AWS backend configuration.
type AwsBackend struct {
	// AccountId is the AWS account ID to use for the upstream.
	// +kubebuilder:validation:Required
	AccountId string `json:"accountId"`
	// Auth specifies the authentication method to use for the upstream.
	// +optional
	Auth *AwsAuth `json:"auth,omitempty"`
	// Lambda configures the AWS lambda service.
	// +optional
	Lambda *AwsLambda `json:"lambda,omitempty"`
	// Region is the AWS region to use for the upstream.
	// Defaults to us-east-1 if not specified.
	// +optional
	Region *string `json:"region,omitempty"`
}

// AwsAuthType is the type of authentication to use for the upstream.
type AwsAuthType string

const (
	// AwsAuthTypeInstanceMetadata is the instance metadata authentication type.
	AwsAuthTypeInstanceMetadata AwsAuthType = "default"
	// AwsAuthTypeIRSA is the IRSA authentication type.
	AwsAuthTypeIRSA AwsAuthType = "irsa"
	// AwsAuthTypeSecret is the secret authentication type.
	AwsAuthTypeSecret AwsAuthType = "secret"
)

// AwsAuth defines the authentication method to use for the backend.
// +kubebuilder:validation:XValidation:message="only one auth method can be specified",rule="!(has(self.irsa) && has(self.secret))"
type AwsAuth struct {
	// IRSA is the IRSA authentication configuration.
	// +optional
	IRSA *AWSAuthIRSA `json:"irsa,omitempty"`
	// Secret is a reference to a secret containing AWS credentials.
	// +optional
	Secret *corev1.LocalObjectReference `json:"secret,omitempty"`
}

// AWSAuthIRSA defines the IRSA configuration for the upstream.
type AWSAuthIRSA struct {
	// RoleARN is the AWS IAM role to assume when using IRSA.
	// Used for IAM-based authentication.
	// +kubebuilder:validation:Required
	RoleARN string `json:"roleARN,omitempty"`
}

const (
	// AwsLambdaInvocationModeSynchronous is the synchronous invocation mode for the lambda function.
	AwsLambdaInvocationModeSynchronous = "Sync"
	// AwsLambdaInvocationModeAsynchronous is the asynchronous invocation mode for the lambda function.
	AwsLambdaInvocationModeAsynchronous = "Async"
)

// AwsLambda configures the AWS lambda service.
type AwsLambda struct {
	// EndpointURL is the URL to use for the upstream host.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	EndpointURL string `json:"endpointURL,omitempty"`
	// FunctionName is the name of the lambda function to invoke.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=140
	FunctionName string `json:"functionName"`
	// InvocationMode defines how to invoke the lambda function.
	// Defaults to SYNCHRONOUS if not specified.
	// +optional
	// +kubebuilder:validation:Enum=Sync;Async
	InvocationMode string `json:"invocationMode,omitempty"`
	// Qualifier is the qualifier of the lambda function to invoke.
	// When unspecified, the $LATEST version of the function is used.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Qualifier string `json:"qualifier,omitempty"`
}

// StaticBackend is an upstream that references a static list of hosts.
type StaticBackend struct {
	// Hosts is a list of hosts to use for the upstream.
	// +kubebuilder:validation:required
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=host
	Hosts []Host `json:"hosts,omitempty"`
}

// Host defines a static upstream host.
type Host struct {
	// Host is the host to use for the upstream.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Host string `json:"host"`
	// Port is the port to use for the upstream.
	// +kubebuilder:validation:Required
	Port gwv1.PortNumber `json:"port"`
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

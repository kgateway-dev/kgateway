package v1alpha1

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ExtProcPolicy struct {
	GrpcService *ExtProcGrpcService `json:"grpcService,omitempty"`

	ProcessingMode *ProcessingMode `json:"processingMode,omitempty"`
}

// ProcessingMode defines how the filter should interact with the request/response streams
type ProcessingMode struct {
	// RequestHeaderMode determines how to handle the request headers
	// +kubebuilder:validation:Enum=DEFAULT;SEND;SKIP
	// +optional
	RequestHeaderMode *string `json:"requestHeaderMode,omitempty"`

	// ResponseHeaderMode determines how to handle the response headers
	// +kubebuilder:validation:Enum=DEFAULT;SEND;SKIP
	// +optional
	ResponseHeaderMode *string `json:"responseHeaderMode,omitempty"`

	// RequestBodyMode determines how to handle the request body
	// +kubebuilder:validation:Enum=NONE;STREAMED;BUFFERED;BUFFERED_PARTIAL;FULL_DUPLEX_STREAMED
	// +optional
	RequestBodyMode *string `json:"requestBodyMode,omitempty"`

	// ResponseBodyMode determines how to handle the response body
	// +kubebuilder:validation:Enum=NONE;STREAMED;BUFFERED;BUFFERED_PARTIAL;FULL_DUPLEX_STREAMED
	// +optional
	ResponseBodyMode *string `json:"responseBodyMode,omitempty"`

	// RequestTrailerMode determines how to handle the request trailers
	// +kubebuilder:validation:Enum=DEFAULT;SEND;SKIP
	// +optional
	RequestTrailerMode *string `json:"requestTrailerMode,omitempty"`

	// ResponseTrailerMode determines how to handle the response trailers
	// +kubebuilder:validation:Enum=DEFAULT;SEND;SKIP
	// +optional
	ResponseTrailerMode *string `json:"responseTrailerMode,omitempty"`
}

type ExtProcGrpcService struct {
	// ExtProcServerRef *corev1.LocalObjectReference `json:"extProcServerRef,omitempty"`

	// The backend gRPC service. Can be any type of supported backed (Kubernetes Service, kgateway Backend, etc..)
	// +kubebuilder:validation:Required
	BackendRef *gwv1.BackendRef `json:"backendRef"`

	Authority *string `json:"authority,omitempty"`
}

// type ExtProcPolicyRouteSettings struct {

// }

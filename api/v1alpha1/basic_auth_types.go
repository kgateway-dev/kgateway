package v1alpha1

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

// BasicAuthPolicy configures HTTP basic authentication using the Authorization header.
// Basic authentication validates requests against username/password pairs provided either inline or via a Kubernetes secret.
// The credentials must be in htpasswd SHA-1 format.
//
// +kubebuilder:validation:ExactlyOneOf=users;secretRef;disable
type BasicAuthPolicy struct {
	// Users provides an inline list of username/password pairs in htpasswd format.
	// Each entry should be formatted as "username:hashed_password".
	// The only supported hash format is SHA-1
	//
	// Example entries:
	//   - "user1:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s="
	//
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	Users []string `json:"users,omitempty"`

	// SecretRef references a Kubernetes secret containing htpasswd data.
	// The secret must contain username/password pairs in htpasswd format.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Disable basic auth.
	// Can be used to disable basic auth policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *PolicyDisable `json:"disable,omitempty"`
}

// SecretReference identifies a Kubernetes secret containing authentication data.
type SecretReference struct {
	// Name of the secret containing htpasswd data.
	// +required
	Name gwv1.ObjectName `json:"name"`

	// Namespace of the secret. If not specified, defaults to the namespace of the TrafficPolicy.
	// Note that a secret in a different namespace requires a ReferenceGrant to be accessible.
	// +optional
	Namespace *gwv1.Namespace `json:"namespace,omitempty"`

	// Key in the secret that contains the htpasswd data.
	// Defaults to ".htpasswd" if not specified.
	// +optional
	// +kubebuilder:default=".htpasswd"
	// +kubebuilder:validation:MinLength=1
	Key *string `json:"key,omitempty"`
}

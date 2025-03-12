// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
// +groupName=gateway.kgateway.dev
package v1alpha1

// Gateway API resources with status management
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses;gateways;httproutes;tcproutes;tlsroutes;referencegrants;backendtlspolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status;gateways/status;httproutes/status;tcproutes/status;tlsroutes/status;backendtlspolicies/status,verbs=patch;update

// Gateway API Inference Extension resources with status management
// +kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencemodels,verbs=get;list;watch
// +kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencepools,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=inference.networking.x-k8s.io,resources=inferencepools/status,verbs=patch;update
// RBAC required for deployer to create RBAC rules required by endpoint picker extension.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;patch;delete;update
// Unsure why endpoint picker extension requires these: https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/224
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Controller resources
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch

// Proxy deployer resources that require extra permissions
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;serviceaccounts,verbs=get;list;watch;create;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;patch;delete

// EDS discovery resources
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch

// CRD access for scheme registration
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Istio resources for traffic management
// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch

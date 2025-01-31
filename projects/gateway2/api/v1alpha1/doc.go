// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
// +groupName=gateway.kgateway.dev
package v1alpha1

// Gateway API resources with status management
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses;gateways;httproutes;tcproutes;referencegrants,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status;gateways/status;httproutes/status;tcproutes/status,verbs=patch;update

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

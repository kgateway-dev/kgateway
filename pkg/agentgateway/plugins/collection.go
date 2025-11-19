package plugins

import (
	"istio.io/istio/pkg/config/schema/gvr"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	kgwversioned "github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type AgwCollections struct {
	OurClient kgwversioned.Interface
	Client    apiclient.Client
	KrtOpts   krtutil.KrtOptions

	// Core Kubernetes resources
	Namespaces         krt.Collection[*corev1.Namespace]
	Nodes              krt.Collection[*corev1.Node]
	Pods               krt.Collection[*corev1.Pod]
	Services           krt.Collection[*corev1.Service]
	Secrets            krt.Collection[*corev1.Secret]
	SecretsByNamespace krt.Index[string, *corev1.Secret]
	ConfigMaps         krt.Collection[*corev1.ConfigMap]
	EndpointSlices     krt.Collection[*discovery.EndpointSlice]

	// Gateway API resources
	GatewayClasses     krt.Collection[*gwv1.GatewayClass]
	Gateways           krt.Collection[*gwv1.Gateway]
	HTTPRoutes         krt.Collection[*gwv1.HTTPRoute]
	GRPCRoutes         krt.Collection[*gwv1.GRPCRoute]
	TCPRoutes          krt.Collection[*gwv1a2.TCPRoute]
	TLSRoutes          krt.Collection[*gwv1a2.TLSRoute]
	ReferenceGrants    krt.Collection[*gwv1b1.ReferenceGrant]
	BackendTLSPolicies krt.Collection[*gwv1.BackendTLSPolicy]
	XListenerSets      krt.Collection[*gwxv1a1.XListenerSet]

	// Extended resources
	InferencePools krt.Collection[*inf.InferencePool]

	// irs shared by common colections and agent gateway
	WrappedPods krt.Collection[krtcollections.WrappedPod]
	RefGrants   *krtcollections.RefGrantIndex

	// kgateway resources
	Backends             krt.Collection[*v1alpha1.Backend]
	AgentgatewayPolicies krt.Collection[*v1alpha1.AgentgatewayPolicy]
	DirectResponses      krt.Collection[*v1alpha1.DirectResponse]
	GatewayExtensions    krt.Collection[*v1alpha1.GatewayExtension]

	// ControllerName is the name of the Gateway controller.
	ControllerName string
	// SystemNamespace is control plane system namespace (default is kgateway-system)
	SystemNamespace string
	// ClusterID is the cluster ID of the cluster the proxy is running in.
	ClusterID string
}

func (c *AgwCollections) HasSynced() bool {
	// we check nil as well because some of the inner
	// collections aren't initialized until we call InitPlugins
	return c.Namespaces != nil && c.Namespaces.HasSynced() &&
		c.Services != nil && c.Services.HasSynced() &&
		c.Secrets != nil && c.Secrets.HasSynced() &&
		c.ConfigMaps != nil && c.ConfigMaps.HasSynced() &&
		c.GatewayClasses != nil && c.GatewayClasses.HasSynced() &&
		c.Gateways != nil && c.Gateways.HasSynced() &&
		c.HTTPRoutes != nil && c.HTTPRoutes.HasSynced() &&
		c.GRPCRoutes != nil && c.GRPCRoutes.HasSynced() &&
		c.TCPRoutes != nil && c.TCPRoutes.HasSynced() &&
		c.TLSRoutes != nil && c.TLSRoutes.HasSynced() &&
		c.ReferenceGrants != nil && c.ReferenceGrants.HasSynced() &&
		c.BackendTLSPolicies != nil && c.BackendTLSPolicies.HasSynced() &&
		c.InferencePools != nil && c.InferencePools.HasSynced() &&
		c.WrappedPods != nil && c.WrappedPods.HasSynced() &&
		c.RefGrants != nil && c.RefGrants.HasSynced() &&
		c.Backends != nil && c.Backends.HasSynced() &&
		c.AgentgatewayPolicies != nil && c.AgentgatewayPolicies.HasSynced() &&
		c.DirectResponses != nil && c.DirectResponses.HasSynced() &&
		c.GatewayExtensions != nil && c.GatewayExtensions.HasSynced()
}

// NewAgwCollections initializes the core krt collections.
// Collections that rely on plugins aren't initialized here,
// and InitPlugins must be called.
func NewAgwCollections(
	commoncol *collections.CommonCollections,
	agwControllerName string,
	systemNamespace string,
	clusterID string,
) (*AgwCollections, error) {
	agwCollections := &AgwCollections{
		Client:          commoncol.Client,
		ControllerName:  agwControllerName,
		SystemNamespace: systemNamespace,
		ClusterID:       clusterID,

		// Core Kubernetes resources
		Namespaces: krt.NewInformer[*corev1.Namespace](commoncol.Client),
		Nodes: krt.NewInformerFiltered[*corev1.Node](commoncol.Client, kclient.Filter{
			ObjectFilter: commoncol.Client.ObjectFilter(),
		}, commoncol.KrtOpts.ToOptions("informer/Nodes")...),
		Pods: krt.NewInformerFiltered[*corev1.Pod](commoncol.Client, kclient.Filter{
			ObjectTransform: istiokube.StripPodUnusedFields,
			ObjectFilter:    commoncol.Client.ObjectFilter(),
		}, commoncol.KrtOpts.ToOptions("informer/Pods")...),

		Secrets: krt.WrapClient(
			kclient.NewFiltered[*corev1.Secret](commoncol.Client, kubetypes.Filter{
				ObjectFilter: commoncol.Client.ObjectFilter(),
			}),
		),
		ConfigMaps: krt.WrapClient(
			kclient.NewFiltered[*corev1.ConfigMap](commoncol.Client, kubetypes.Filter{
				ObjectFilter: commoncol.Client.ObjectFilter(),
			}),
			commoncol.KrtOpts.ToOptions("informer/ConfigMaps")...,
		),
		Services: krt.WrapClient(
			kclient.NewFiltered[*corev1.Service](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}),
			commoncol.KrtOpts.ToOptions("informer/Services")...),
		EndpointSlices: krt.WrapClient(
			kclient.NewFiltered[*discovery.EndpointSlice](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}),
			commoncol.KrtOpts.ToOptions("informer/EndpointSlices")...),

		// Gateway API resources
		GatewayClasses:     krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.GatewayClass](commoncol.Client, wellknown.GatewayClassGVR, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/GatewayClasses")...),
		Gateways:           krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.Gateway](commoncol.Client, wellknown.GatewayGVR, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/Gateways")...),
		HTTPRoutes:         krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.HTTPRoute](commoncol.Client, wellknown.HTTPRouteGVR, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/HTTPRoutes")...),
		GRPCRoutes:         krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.GRPCRoute](commoncol.Client, wellknown.GRPCRouteGVR, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/GRPCRoutes")...),
		BackendTLSPolicies: krt.WrapClient(kclient.NewDelayedInformer[*gwv1.BackendTLSPolicy](commoncol.Client, gvr.BackendTLSPolicy, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/BackendTLSPolicies")...),

		// Gateway API alpha
		TCPRoutes:       krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](commoncol.Client, gvr.TCPRoute, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/TCPRoutes")...),
		TLSRoutes:       krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TLSRoute](commoncol.Client, gvr.TLSRoute, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/TLSRoutes")...),
		ReferenceGrants: krt.WrapClient(kclient.NewFilteredDelayed[*gwv1b1.ReferenceGrant](commoncol.Client, wellknown.ReferenceGrantGVR, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/ReferenceGrants")...),
		XListenerSets:   krt.WrapClient(kclient.NewDelayedInformer[*gwxv1a1.XListenerSet](commoncol.Client, gvr.XListenerSet, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/XListenerSets")...),
		// BackendTrafficPolicy?

		// inference extensions need to be enabled so control plane has permissions to watch resource. Disable by default
		InferencePools: krt.NewStaticCollection[*inf.InferencePool](nil, nil, commoncol.KrtOpts.ToOptions("disable/inferencepools")...),

		// common collections
		WrappedPods: commoncol.WrappedPods,
		RefGrants:   commoncol.RefGrants,

		// kgateway resources
		DirectResponses:      krt.NewInformer[*v1alpha1.DirectResponse](commoncol.Client),
		AgentgatewayPolicies: krt.NewInformer[*v1alpha1.AgentgatewayPolicy](commoncol.Client),
		GatewayExtensions:    krt.NewInformer[*v1alpha1.GatewayExtension](commoncol.Client),
		Backends:             krt.NewInformer[*v1alpha1.Backend](commoncol.Client),
	}

	if commoncol.Settings.EnableInferExt {
		// inference extensions cluster watch permissions are controlled by enabling EnableInferExt
		inferencePoolGVR := wellknown.InferencePoolGVK.GroupVersion().WithResource("inferencepools")
		agwCollections.InferencePools = krt.WrapClient(kclient.NewDelayedInformer[*inf.InferencePool](commoncol.Client, inferencePoolGVR, kubetypes.StandardInformer, kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/InferencePools")...)
	}
	agwCollections.SecretsByNamespace = krt.NewNamespaceIndex(agwCollections.Secrets)

	return agwCollections, nil
}

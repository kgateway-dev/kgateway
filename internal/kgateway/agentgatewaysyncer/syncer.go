package agentgatewaysyncer

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/avast/retry-go/v4"
	envoytypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	"sigs.k8s.io/gateway-api-inference-extension/client-go/clientset/versioned"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	kgwversioned "github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

var logger = logging.New("agentgateway/syncer")

const (
	// Retry configuration constants
	maxRetryAttempts = 5
	retryDelay       = 100 * time.Millisecond

	// Resource name format strings
	resourceNameFormat = "%s~%s"
	bindKeyFormat      = "%s/%s"
	gatewayNameFormat  = "%s/%s"

	// Log message keys
	logKeyControllerName = "controllername"
	logKeyError          = "error"
	logKeyGateway        = "gateway"
	logKeyResourceRef    = "resource_ref"
	logKeyRouteType      = "route_type"
)

// AgentGwSyncer synchronizes Kubernetes Gateway API resources with xDS for agentgateway proxies.
// It watches Gateway resources with the agentgateway class and translates them to agentgateway configuration.
type AgentGwSyncer struct {
	// Core collections and dependencies
	commonCols *common.CommonCollections
	mgr        manager.Manager
	client     kube.Client
	plugins    pluginsdk.Plugin
	translator *translator.AgentGatewayTranslator

	// Configuration
	controllerName        string
	agentGatewayClassName string
	systemNamespace       string
	clusterID             string

	// XDS and caching
	xDS      krt.Collection[agentGwXdsResources]
	xdsCache envoycache.SnapshotCache

	// Status reporting
	gatewayReports     krt.Singleton[GatewayReports]
	listenerSetReports krt.Singleton[ListenerSetReports]
	routeReports       krt.Singleton[RouteReports]

	// Synchronization
	waitForSync []cache.InformerSynced
	ready       atomic.Bool

	// features
	EnableInferExt bool
}

// agentGwXdsResources represents XDS resources for a single agent gateway
type agentGwXdsResources struct {
	types.NamespacedName

	// Status reports for this gateway
	reports        reports.ReportMap
	attachedRoutes map[string]uint

	// Resources config for gateway (Bind, Listener, Route)
	ResourceConfig envoycache.Resources

	// Address config (Services, Workloads)
	AddressConfig envoycache.Resources
}

// ResourceName needs to match agentgateway role configured in agentgateway
func (r agentGwXdsResources) ResourceName() string {
	return fmt.Sprintf(resourceNameFormat, r.Namespace, r.Name)
}

func (r agentGwXdsResources) Equals(in agentGwXdsResources) bool {
	return r.NamespacedName == in.NamespacedName &&
		report{reportMap: r.reports, attachedRoutes: r.attachedRoutes}.Equals(report{reportMap: in.reports, attachedRoutes: in.attachedRoutes}) &&
		r.ResourceConfig.Version == in.ResourceConfig.Version &&
		r.AddressConfig.Version == in.AddressConfig.Version
}

func NewAgentGwSyncer(
	controllerName string,
	agentGatewayClassName string,
	client kube.Client,
	mgr manager.Manager,
	commonCols *common.CommonCollections,
	plugins pluginsdk.Plugin,
	xdsCache envoycache.SnapshotCache,
	systemNamespace string,
	clusterID string,
	enableInferExt bool,
) *AgentGwSyncer {
	return &AgentGwSyncer{
		commonCols:            commonCols,
		controllerName:        controllerName,
		agentGatewayClassName: agentGatewayClassName,
		plugins:               plugins,
		translator:            translator.NewAgentGatewayTranslator(commonCols, plugins),
		xdsCache:              xdsCache,
		client:                client,
		mgr:                   mgr,
		systemNamespace:       systemNamespace,
		clusterID:             clusterID,
		EnableInferExt:        enableInferExt,
	}
}

type envoyResourceWithCustomName struct {
	proto.Message
	Name    string
	version uint64
}

func (r envoyResourceWithCustomName) ResourceName() string {
	return r.Name
}

func (r envoyResourceWithCustomName) GetName() string {
	return r.Name
}

func (r envoyResourceWithCustomName) Equals(in envoyResourceWithCustomName) bool {
	return r.version == in.version
}

var _ envoytypes.ResourceWithName = envoyResourceWithCustomName{}

type report struct {
	// lower case so krt doesn't error in debug handler
	reportMap      reports.ReportMap
	attachedRoutes map[string]uint
}

// RouteReports contains all route-related reports
type RouteReports struct {
	HTTPRoutes map[types.NamespacedName]*reports.RouteReport
	GRPCRoutes map[types.NamespacedName]*reports.RouteReport
	TCPRoutes  map[types.NamespacedName]*reports.RouteReport
	TLSRoutes  map[types.NamespacedName]*reports.RouteReport
}

func (r RouteReports) ResourceName() string {
	return "route-reports"
}

func (r RouteReports) Equals(in RouteReports) bool {
	return maps.Equal(r.HTTPRoutes, in.HTTPRoutes) &&
		maps.Equal(r.GRPCRoutes, in.GRPCRoutes) &&
		maps.Equal(r.TCPRoutes, in.TCPRoutes) &&
		maps.Equal(r.TLSRoutes, in.TLSRoutes)
}

// ListenerSetReports contains all listener set reports
type ListenerSetReports struct {
	Reports map[types.NamespacedName]*reports.ListenerSetReport
}

func (l ListenerSetReports) ResourceName() string {
	return "listenerset-reports"
}

func (l ListenerSetReports) Equals(in ListenerSetReports) bool {
	return maps.Equal(l.Reports, in.Reports)
}

// GatewayReports contains gateway reports along with attached routes information
type GatewayReports struct {
	Reports        map[types.NamespacedName]*reports.GatewayReport
	AttachedRoutes map[types.NamespacedName]map[string]uint
}

func (g GatewayReports) ResourceName() string {
	return "gateway-reports"
}

func (g GatewayReports) Equals(in GatewayReports) bool {
	if !maps.Equal(g.Reports, in.Reports) {
		return false
	}

	// Compare AttachedRoutes manually since it contains nested maps
	if len(g.AttachedRoutes) != len(in.AttachedRoutes) {
		return false
	}
	for key, gRoutes := range g.AttachedRoutes {
		inRoutes, exists := in.AttachedRoutes[key]
		if !exists {
			return false
		}
		if !maps.Equal(gRoutes, inRoutes) {
			return false
		}
	}

	return true
}

func (r report) ResourceName() string {
	return "report"
}

func (r report) Equals(in report) bool {
	if !maps.Equal(r.reportMap.Gateways, in.reportMap.Gateways) {
		return false
	}
	if !maps.Equal(r.reportMap.ListenerSets, in.reportMap.ListenerSets) {
		return false
	}
	if !maps.Equal(r.reportMap.HTTPRoutes, in.reportMap.HTTPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TCPRoutes, in.reportMap.TCPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TLSRoutes, in.reportMap.TLSRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.Policies, in.reportMap.Policies) {
		return false
	}
	if !maps.Equal(r.attachedRoutes, in.attachedRoutes) {
		return false
	}
	return true
}

// Inputs holds all the input collections needed for the syncer
type Inputs struct {
	// Core Kubernetes resources
	Namespaces krt.Collection[*corev1.Namespace]
	Services   krt.Collection[*corev1.Service]
	Secrets    krt.Collection[*corev1.Secret]

	// Gateway API resources
	GatewayClasses  krt.Collection[*gwv1.GatewayClass]
	Gateways        krt.Collection[*gwv1.Gateway]
	HTTPRoutes      krt.Collection[*gwv1.HTTPRoute]
	GRPCRoutes      krt.Collection[*gwv1.GRPCRoute]
	TCPRoutes       krt.Collection[*gwv1alpha2.TCPRoute]
	TLSRoutes       krt.Collection[*gwv1alpha2.TLSRoute]
	ReferenceGrants krt.Collection[*gwv1beta1.ReferenceGrant]

	// Extended resources
	ServiceEntries krt.Collection[*networkingclient.ServiceEntry]
	InferencePools krt.Collection[*inf.InferencePool]

	// kgateway resources
	Backends *krtcollections.BackendIndex
}

func (s *AgentGwSyncer) Init(krtopts krtinternal.KrtOptions) {
	logger.Debug("init agentgateway Syncer", "controllername", s.controllerName)

	s.translator.Init()

	inputs := s.buildInputCollections(krtopts)

	s.setupkgwResources(s.commonCols.OurClient)
	s.setupInferenceExtensionClient()

	finalBackends, _ := s.buildBackendCollections(inputs, krtopts)

	// Pass finalBackends into buildResourceCollections instead of storing on syncer
	s.buildResourceCollections(inputs, finalBackends, krtopts)
}

func (s *AgentGwSyncer) setupkgwResources(kgwClient kgwversioned.Interface) {
	kubeclient.Register[*v1alpha1.Backend](
		wellknown.BackendGVR,
		wellknown.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return kgwClient.GatewayV1alpha1().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return kgwClient.GatewayV1alpha1().Backends(namespace).Watch(context.Background(), o)
		},
	)
}

func (s *AgentGwSyncer) setupInferenceExtensionClient() {
	// TODO: share this in a common spot with the inference extension plugin
	// Create the inference extension clientset.
	inferencePoolGVR := wellknown.InferencePoolGVK.GroupVersion().WithResource("inferencepools")
	infCli, err := versioned.NewForConfig(s.commonCols.Client.RESTConfig())
	if err != nil {
		logger.Error("failed to create inference extension client", "error", err)
	} else {
		kubeclient.Register[*inf.InferencePool](
			inferencePoolGVR,
			wellknown.InferencePoolGVK,
			func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
				return infCli.InferenceV1alpha2().InferencePools(namespace).List(context.Background(), o)
			},
			func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
				return infCli.InferenceV1alpha2().InferencePools(namespace).Watch(context.Background(), o)
			},
		)
	}
}

func (s *AgentGwSyncer) buildInputCollections(krtopts krtinternal.KrtOptions) Inputs {
	inputs := Inputs{
		Namespaces: krt.NewInformer[*corev1.Namespace](s.client),
		Secrets: krt.WrapClient[*corev1.Secret](
			kclient.NewFiltered[*corev1.Secret](s.client, kubetypes.Filter{
				//FieldSelector: kubesecrets.SecretsFieldSelector,
				ObjectFilter: s.client.ObjectFilter(),
			}),
		),
		Services: krt.WrapClient[*corev1.Service](
			kclient.NewFiltered[*corev1.Service](s.client, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}),
			krtopts.ToOptions("informer/Services")...),

		GatewayClasses: krt.WrapClient(kclient.NewFiltered[*gwv1.GatewayClass](s.client, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/GatewayClasses")...),
		Gateways:       krt.WrapClient(kclient.NewFiltered[*gwv1.Gateway](s.client, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/Gateways")...),
		HTTPRoutes:     krt.WrapClient(kclient.NewFiltered[*gwv1.HTTPRoute](s.client, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/HTTPRoutes")...),
		GRPCRoutes:     krt.WrapClient(kclient.NewFiltered[*gwv1.GRPCRoute](s.client, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/GRPCRoutes")...),

		ReferenceGrants: krt.WrapClient(kclient.NewFiltered[*gwv1beta1.ReferenceGrant](s.client, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/ReferenceGrants")...),
		//ServiceEntries:  krt.WrapClient(kclient.New[*networkingclient.ServiceEntry](s.client), krtopts.ToOptions("informer/ServiceEntries")...),

		// kubernetes gateway alpha apis
		TCPRoutes: krt.WrapClient(kclient.NewDelayedInformer[*gwv1alpha2.TCPRoute](s.client, gvr.TCPRoute, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/TCPRoutes")...),
		TLSRoutes: krt.WrapClient(kclient.NewDelayedInformer[*gwv1alpha2.TLSRoute](s.client, gvr.TLSRoute, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: s.client.ObjectFilter()}), krtopts.ToOptions("informer/TLSRoutes")...),

		// inference extensions need to be enabled so control plane has permissions to watch resource. Disable by default
		InferencePools: krt.NewStaticCollection[*inf.InferencePool](nil, nil, krtopts.ToOptions("disable/inferencepools")...),

		// kgateway resources
		Backends: s.commonCols.BackendIndex,
	}

	if s.EnableInferExt {
		// inference extensions cluster watch permissions are controlled by enabling EnableInferExt
		inputs.InferencePools = krt.WrapClient(kclient.NewDelayedInformer[*inf.InferencePool](s.client, gvr.InferencePool, kubetypes.StandardInformer, kclient.Filter{ObjectFilter: s.commonCols.Client.ObjectFilter()}), krtopts.ToOptions("informer/InferencePools")...)
	}

	return inputs
}

func (s *AgentGwSyncer) buildResourceCollections(inputs Inputs, finalBackends krt.Collection[ir.BackendObjectIR], krtopts krtinternal.KrtOptions) {
	// Build core collections for irs
	gatewayClasses := GatewayClassesCollection(inputs.GatewayClasses, krtopts)
	refGrants := BuildReferenceGrants(ReferenceGrantsCollection(inputs.ReferenceGrants, krtopts))
	gateways := s.buildGatewayCollection(inputs, gatewayClasses, refGrants, krtopts)

	// Build ADP resources for gateway
	adpResources := s.buildADPResources(gateways, inputs, refGrants, krtopts)

	// Create ADP backend collection from finalBackends
	adpBackends := s.newADPBackendCollection(inputs, finalBackends, krtopts)

	// Build address collections
	addresses := s.buildAddressCollections(inputs, krtopts)

	// Build XDS collection
	s.buildXDSCollection(adpResources, adpBackends, addresses, krtopts)

	// Build status reporting
	s.buildStatusReporting()

	// Set up sync dependencies
	s.setupSyncDependencies(gateways, adpResources, adpBackends, addresses, inputs)
}

func (s *AgentGwSyncer) buildGatewayCollection(
	inputs Inputs,
	gatewayClasses krt.Collection[GatewayClass],
	refGrants ReferenceGrants,
	krtopts krtinternal.KrtOptions,
) krt.Collection[GatewayListener] {
	return GatewayCollection(
		s.agentGatewayClassName,
		inputs.Gateways,
		gatewayClasses,
		inputs.Namespaces,
		refGrants,
		inputs.Secrets,
		krtopts,
	)
}

func (s *AgentGwSyncer) buildADPResources(
	gateways krt.Collection[GatewayListener],
	inputs Inputs,
	refGrants ReferenceGrants,
	krtopts krtinternal.KrtOptions,
) krt.Collection[ADPResourcesForGateway] {
	// Build ports and binds
	ports := krtpkg.UnnamedIndex(gateways, func(l GatewayListener) []string {
		return []string{fmt.Sprint(l.parentInfo.Port)}
	}).AsCollection(krtopts.ToOptions("PortBindings")...)

	binds := krt.NewManyCollection(ports, func(ctx krt.HandlerContext, object krt.IndexObject[string, GatewayListener]) []ADPResourcesForGateway {
		port, _ := strconv.Atoi(object.Key)
		gwReports := make(map[types.NamespacedName]reports.ReportMap, 0)
		for _, gw := range object.Objects {
			key := types.NamespacedName{
				Namespace: gw.parent.Namespace,
				Name:      gw.parent.Name,
			}
			gwReports[key] = gw.report
		}
		var results []ADPResourcesForGateway
		binds := make(map[types.NamespacedName][]*api.Resource)
		for nsName := range gwReports {
			bind := ADPBind{
				Bind: &api.Bind{
					Key:  object.Key + "/" + nsName.String(),
					Port: uint32(port),
				},
			}
			if binds[nsName] == nil {
				binds[nsName] = make([]*api.Resource, 0)
			}
			binds[nsName] = append(binds[nsName], toADPResource(bind))
		}
		for gw, res := range binds {
			repForGw := gwReports[gw]
			results = append(results, toResourceWithRoutes(gw, res, nil, repForGw))
		}
		return results
	}, krtopts.ToOptions("Binds")...)

	// Build listeners
	listeners := krt.NewCollection(gateways, func(ctx krt.HandlerContext, obj GatewayListener) *ADPResourcesForGateway {
		return s.buildListenerFromGateway(obj)
	}, krtopts.ToOptions("Listeners")...)

	// Build routes
	routeParents := BuildRouteParents(gateways)
	routeInputs := RouteContextInputs{
		Grants:         refGrants,
		RouteParents:   routeParents,
		Services:       inputs.Services,
		Namespaces:     inputs.Namespaces,
		InferencePools: inputs.InferencePools,
		Backends:       s.commonCols.BackendIndex,
		Plugins:        s.plugins,
	}
	adpRoutes := ADPRouteCollection(inputs.HTTPRoutes, inputs.GRPCRoutes, inputs.TCPRoutes, inputs.TLSRoutes, routeInputs, krtopts, s.plugins)

	adpPolicies := ADPPolicyCollection(inputs, binds, krtopts)

	// Join all ADP resources
	allADPResources := krt.JoinCollection([]krt.Collection[ADPResourcesForGateway]{binds, listeners, adpRoutes, adpPolicies}, krtopts.ToOptions("ADPResources")...)

	return allADPResources
}

// buildListenerFromGateway creates a listener resource from a gateway
func (s *AgentGwSyncer) buildListenerFromGateway(obj GatewayListener) *ADPResourcesForGateway {
	l := &api.Listener{
		Key:         obj.ResourceName(),
		Name:        string(obj.parentInfo.SectionName),
		BindKey:     fmt.Sprint(obj.parentInfo.Port) + "/" + obj.parent.Namespace + "/" + obj.parent.Name,
		GatewayName: obj.parent.Namespace + "/" + obj.parent.Name,
		Hostname:    obj.parentInfo.OriginalHostname,
	}

	// Set protocol and TLS configuration
	protocol, tlsConfig, ok := s.getProtocolAndTLSConfig(obj)
	if !ok {
		return nil // Unsupported protocol or missing TLS config
	}

	l.Protocol = protocol
	l.Tls = tlsConfig

	resources := []*api.Resource{toADPResource(ADPListener{l})}
	return toResourcep(types.NamespacedName{
		Namespace: obj.parent.Namespace,
		Name:      obj.parent.Name,
	}, resources, obj.report)
}

// buildBackendFromBackendIR creates a backend resource from BackendObjectIR
func (s *AgentGwSyncer) buildBackendFromBackendIR(ctx krt.HandlerContext, backendIR *ir.BackendObjectIR, svcCol krt.Collection[*corev1.Service], secretsCol krt.Collection[*corev1.Secret], nsCol krt.Collection[*corev1.Namespace]) []envoyResourceWithCustomName {
	var results []envoyResourceWithCustomName
	backends, backendPolicies, err := s.translator.BackendTranslator().TranslateBackend(ctx, backendIR, svcCol, secretsCol, nsCol)
	if err != nil {
		logger.Error("failed to translate backend", "backend", backendIR.Name, "namespace", backendIR.Namespace, "error", err)
		return results
	}
	// handle all backends created as an MCP backend may create multiple backends
	for _, backend := range backends {
		logger.Debug("creating backend", "backend", backend.Name)
		resourceWrapper := &api.Resource{
			Kind: &api.Resource_Backend{
				Backend: backend,
			},
		}
		results = append(results, envoyResourceWithCustomName{
			Message: resourceWrapper,
			Name:    backend.Name,
			version: utils.HashProto(resourceWrapper),
		})
	}
	for _, policy := range backendPolicies {
		logger.Debug("creating backend policy", "policy", policy.Name)
		resourceWrapper := &api.Resource{
			Kind: &api.Resource_Policy{
				Policy: policy,
			},
		}
		results = append(results, envoyResourceWithCustomName{
			Message: resourceWrapper,
			Name:    policy.Name,
			version: utils.HashProto(resourceWrapper),
		})
	}
	return results
}

// newADPBackendCollection creates the ADP backend collection for agent gateway resources
func (s *AgentGwSyncer) newADPBackendCollection(inputs Inputs, finalBackends krt.Collection[ir.BackendObjectIR], krtopts krtinternal.KrtOptions) krt.Collection[envoyResourceWithCustomName] {
	backends := krt.NewManyCollection(finalBackends, func(ctx krt.HandlerContext, backendIR ir.BackendObjectIR) []envoyResourceWithCustomName {
		if backendIR.Group == wellknown.ServiceGVK.Group && backendIR.Kind == wellknown.ServiceGVK.Kind {
			return nil
		}
		return s.buildBackendFromBackendIR(ctx, &backendIR, inputs.Services, inputs.Secrets, inputs.Namespaces)
	}, krtopts.ToOptions("ADPBackends")...)

	return backends
}

// buildBackendCollections builds the filtered backend IR collection and the corresponding ADP backend collection
func (s *AgentGwSyncer) buildBackendCollections(
	inputs Inputs,
	krtopts krtinternal.KrtOptions,
) (krt.Collection[ir.BackendObjectIR], krt.Collection[envoyResourceWithCustomName]) {
	// Get all backends with attached policies, filtering out Service backends
	// Agent gateway handles Service references directly in routes and doesn't need separate backend objects
	allBackends := krt.JoinCollection(s.commonCols.BackendIndex.BackendsWithPolicy(),
		append(krtopts.ToOptions("AllBackends"), krt.WithJoinUnchecked())...)

	finalBackends := krt.NewCollection(allBackends, func(kctx krt.HandlerContext, backend *ir.BackendObjectIR) *ir.BackendObjectIR {
		if backend.Group == wellknown.ServiceGVK.Group && backend.Kind == wellknown.ServiceGVK.Kind {
			return nil
		}
		return backend
	}, krtopts.ToOptions("FinalBackends")...)

	adpBackends := s.newADPBackendCollection(inputs, finalBackends, krtopts)
	return finalBackends, adpBackends
}

// getProtocolAndTLSConfig extracts protocol and TLS configuration from a gateway
func (s *AgentGwSyncer) getProtocolAndTLSConfig(obj GatewayListener) (api.Protocol, *api.TLSConfig, bool) {
	var tlsConfig *api.TLSConfig

	// Build TLS config if needed
	if obj.TLSInfo != nil {
		tlsConfig = &api.TLSConfig{
			Cert:       obj.TLSInfo.Cert,
			PrivateKey: obj.TLSInfo.Key,
		}
	}

	switch obj.parentInfo.Protocol {
	case gwv1.HTTPProtocolType:
		return api.Protocol_HTTP, nil, true
	case gwv1.HTTPSProtocolType:
		if tlsConfig == nil {
			return api.Protocol_HTTPS, nil, false // TLS required but not configured
		}
		return api.Protocol_HTTPS, tlsConfig, true
	case gwv1.TLSProtocolType:
		if tlsConfig == nil {
			return api.Protocol_TLS, nil, false // TLS required but not configured
		}
		return api.Protocol_TLS, tlsConfig, true
	case gwv1.TCPProtocolType:
		return api.Protocol_TCP, nil, true
	default:
		return api.Protocol_HTTP, nil, false // Unsupported protocol
	}
}

func (s *AgentGwSyncer) buildAddressCollections(inputs Inputs, krtopts krtinternal.KrtOptions) krt.Collection[envoyResourceWithCustomName] {
	// Build endpoint slices and namespaces
	epSliceClient := kclient.NewFiltered[*discoveryv1.EndpointSlice](
		s.commonCols.Client,
		kclient.Filter{ObjectFilter: s.commonCols.Client.ObjectFilter()},
	)
	endpointSlices := krt.WrapClient(epSliceClient, s.commonCols.KrtOpts.ToOptions("informer/EndpointSlices")...)

	nsClient := kclient.NewFiltered[*corev1.Namespace](
		s.commonCols.Client,
		kclient.Filter{ObjectFilter: s.commonCols.Client.ObjectFilter()},
	)
	namespaces := krt.WrapClient(nsClient, s.commonCols.KrtOpts.ToOptions("informer/Namespaces")...)

	// Build workload index
	workloadIndex := index{
		namespaces:      s.commonCols.Namespaces,
		SystemNamespace: s.systemNamespace,
		ClusterID:       s.clusterID,
	}

	// Build service and workload collections
	workloadServices := workloadIndex.ServicesCollection(inputs.Services, nil, inputs.InferencePools, namespaces, krtopts)
	workloads := workloadIndex.WorkloadsCollection(
		s.commonCols.WrappedPods,
		workloadServices,
		nil, // serviceEntries,
		endpointSlices,
		namespaces,
		krtopts,
	)

	// Build address collections
	svcAddresses := krt.NewCollection(workloadServices, func(ctx krt.HandlerContext, obj ServiceInfo) *ADPCacheAddress {
		addrMessage := obj.AsAddress.Address
		resourceVersion := utils.HashProto(addrMessage)
		result := &ADPCacheAddress{
			NamespacedName:      types.NamespacedName{Name: obj.Service.GetName(), Namespace: obj.Service.GetNamespace()},
			Address:             addrMessage,
			AddressResourceName: obj.ResourceName(),
			AddressVersion:      resourceVersion,
		}
		logger.Debug("created XDS resources for svc address with ID", "addr", fmt.Sprintf("%s,%s", obj.Service.GetName(), obj.Service.GetNamespace()), "resourceid", result.ResourceName())
		return result
	})

	workloadAddresses := krt.NewCollection(workloads, func(ctx krt.HandlerContext, obj WorkloadInfo) *ADPCacheAddress {
		addrMessage := obj.AsAddress.Address
		resourceVersion := utils.HashProto(addrMessage)
		result := &ADPCacheAddress{
			NamespacedName:      types.NamespacedName{Name: obj.Workload.GetName(), Namespace: obj.Workload.GetNamespace()},
			Address:             addrMessage,
			AddressVersion:      resourceVersion,
			AddressResourceName: obj.ResourceName(),
		}
		logger.Debug("created XDS resources for workload address with ID", "addr", fmt.Sprintf("%s,%s", obj.Workload.GetName(), obj.Workload.GetNamespace()), "resourceid", result.ResourceName())
		return result
	})

	adpAddresses := krt.JoinCollection([]krt.Collection[ADPCacheAddress]{svcAddresses, workloadAddresses}, krtopts.ToOptions("ADPAddresses")...)
	return krt.NewCollection(adpAddresses, func(kctx krt.HandlerContext, obj ADPCacheAddress) *envoyResourceWithCustomName {
		return &envoyResourceWithCustomName{
			Message: obj.Address,
			Name:    obj.AddressResourceName,
			version: obj.AddressVersion,
		}
	}, krtopts.ToOptions("XDSAddresses")...)
}

func (s *AgentGwSyncer) buildXDSCollection(
	adpResources krt.Collection[ADPResourcesForGateway],
	adpBackends krt.Collection[envoyResourceWithCustomName],
	xdsAddresses krt.Collection[envoyResourceWithCustomName],
	krtopts krtinternal.KrtOptions,
) {
	// Create an index on adpResources by Gateway to avoid fetching all resources
	adpResourcesByGateway := krt.NewIndex(adpResources, "gateway", func(resource ADPResourcesForGateway) []types.NamespacedName {
		return []types.NamespacedName{resource.Gateway}
	})

	s.xDS = krt.NewCollection(adpResources, func(kctx krt.HandlerContext, obj ADPResourcesForGateway) *agentGwXdsResources {
		gwNamespacedName := obj.Gateway

		cacheAddresses := krt.Fetch(kctx, xdsAddresses)
		envoytypesAddresses := make([]envoytypes.Resource, 0, len(cacheAddresses))
		for _, addr := range cacheAddresses {
			envoytypesAddresses = append(envoytypesAddresses, &addr)
		}

		// Create a copy of the shared ReportMap to avoid concurrent modification
		gwReports := reports.NewReportMap()

		var cacheResources []envoytypes.Resource
		attachedRoutes := make(map[string]uint)
		// Use index to fetch only resources for this gateway instead of all resources
		resourceList := krt.Fetch(kctx, adpResources, krt.FilterIndex(adpResourcesByGateway, gwNamespacedName))
		for _, resource := range resourceList {
			// 1. merge GW Reports for all Proxies' status reports
			maps.Copy(gwReports.Gateways, resource.report.Gateways)

			// 2. merge LS Reports for all Proxies' status reports
			maps.Copy(gwReports.ListenerSets, resource.report.ListenerSets)

			// 3. merge route parentRefs into RouteReports for all route types
			mergeRouteReports(gwReports.HTTPRoutes, resource.report.HTTPRoutes)
			mergeRouteReports(gwReports.TCPRoutes, resource.report.TCPRoutes)
			mergeRouteReports(gwReports.TLSRoutes, resource.report.TLSRoutes)
			mergeRouteReports(gwReports.GRPCRoutes, resource.report.GRPCRoutes)

			for key, rr := range resource.report.Policies {
				// if we haven't encountered this policy, just copy it over completely
				old := gwReports.Policies[key]
				if old == nil {
					gwReports.Policies[key] = rr
					continue
				}
				// else, let's merge our parentRefs into the existing map
				// obsGen will stay as-is...
				maps.Copy(gwReports.Policies[key].Ancestors, rr.Ancestors)
			}

			for _, res := range resource.Resources {
				cacheResources = append(cacheResources, &envoyResourceWithCustomName{
					Message: res,
					Name:    getADPResourceName(res),
					version: utils.HashProto(res),
				})
				for listenerName, count := range resource.attachedRoutes {
					attachedRoutes[listenerName] += count
				}
			}
		}

		// Fetch all backends and add them to the resources for every gateway
		cachedBackends := krt.Fetch(kctx, adpBackends)
		for _, backend := range cachedBackends {
			cacheResources = append(cacheResources, &backend)
		}

		// Create the resource wrappers
		var resourceVersion uint64
		for _, res := range cacheResources {
			resourceVersion ^= res.(*envoyResourceWithCustomName).version
		}
		// Calculate address version
		var addrVersion uint64
		for _, res := range cacheAddresses {
			addrVersion ^= res.version
		}

		result := &agentGwXdsResources{
			NamespacedName: gwNamespacedName,
			reports:        gwReports,
			attachedRoutes: attachedRoutes,
			ResourceConfig: envoycache.NewResources(fmt.Sprintf("%d", resourceVersion), cacheResources),
			AddressConfig:  envoycache.NewResources(fmt.Sprintf("%d", addrVersion), envoytypesAddresses),
		}
		logger.Debug("created XDS resources for gateway with ID", "gwname", fmt.Sprintf("%s,%s", gwNamespacedName.Name, gwNamespacedName.Namespace), "resourceid", result.ResourceName())
		return result
	}, krtopts.ToOptions("agent-xds")...)
}

func (s *AgentGwSyncer) buildStatusReporting() {
	// TODO(npolshak): Move away from report map and separately fetch resource reports
	// Create separate singleton collections for each resource type instead of merging everything
	// This avoids the overhead of creating and processing a single large merged report
	gatewayReports := krt.NewSingleton(func(kctx krt.HandlerContext) *GatewayReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := make(map[types.NamespacedName]*reports.GatewayReport)
		attachedRoutes := make(map[types.NamespacedName]map[string]uint)

		for _, p := range proxies {
			// Merge GW Reports for all Proxies' status reports
			maps.Copy(merged, p.reports.Gateways)

			// Collect attached routes for each gateway
			if attachedRoutes[p.NamespacedName] == nil {
				attachedRoutes[p.NamespacedName] = make(map[string]uint)
			}
			for listener, counts := range p.attachedRoutes {
				attachedRoutes[p.NamespacedName][listener] += counts
			}
		}

		return &GatewayReports{
			Reports:        merged,
			AttachedRoutes: attachedRoutes,
		}
	})

	listenerSetReports := krt.NewSingleton(func(kctx krt.HandlerContext) *ListenerSetReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := make(map[types.NamespacedName]*reports.ListenerSetReport)

		for _, p := range proxies {
			// Merge LS Reports for all Proxies' status reports
			maps.Copy(merged, p.reports.ListenerSets)
		}

		return &ListenerSetReports{
			Reports: merged,
		}
	})

	routeReports := krt.NewSingleton(func(kctx krt.HandlerContext) *RouteReports {
		proxies := krt.Fetch(kctx, s.xDS)
		merged := RouteReports{
			HTTPRoutes: make(map[types.NamespacedName]*reports.RouteReport),
			GRPCRoutes: make(map[types.NamespacedName]*reports.RouteReport),
			TCPRoutes:  make(map[types.NamespacedName]*reports.RouteReport),
			TLSRoutes:  make(map[types.NamespacedName]*reports.RouteReport),
		}

		for _, p := range proxies {
			// Merge route parentRefs into RouteReports for all route types
			mergeRouteReports(merged.HTTPRoutes, p.reports.HTTPRoutes)
			mergeRouteReports(merged.GRPCRoutes, p.reports.GRPCRoutes)
			mergeRouteReports(merged.TCPRoutes, p.reports.TCPRoutes)
			mergeRouteReports(merged.TLSRoutes, p.reports.TLSRoutes)
		}

		return &merged
	})

	// Store references to the separate collections
	s.gatewayReports = gatewayReports
	s.listenerSetReports = listenerSetReports
	s.routeReports = routeReports
}

func (s *AgentGwSyncer) setupSyncDependencies(gateways krt.Collection[GatewayListener], adpResources krt.Collection[ADPResourcesForGateway], adpBackends krt.Collection[envoyResourceWithCustomName], addresses krt.Collection[envoyResourceWithCustomName], inputs Inputs) {
	s.waitForSync = []cache.InformerSynced{
		s.commonCols.HasSynced,
		gateways.HasSynced,
		// resources
		adpResources.HasSynced,
		adpBackends.HasSynced,
		s.xDS.HasSynced,
		// addresses
		addresses.HasSynced,
		inputs.Namespaces.HasSynced,
	}
}

func (s *AgentGwSyncer) Start(ctx context.Context) error {
	logger.Info("starting agentgateway Syncer", "controllername", s.controllerName)
	logger.Info("waiting for agentgateway cache to sync")

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.client.WaitForCacheSync(
		"kube gw proxy syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("kube gateway sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// Create separate queues for each resource type to avoid processing the entire reportMap
	gatewayReportQueue := utils.NewAsyncQueue[GatewayReports]()
	listenerSetReportQueue := utils.NewAsyncQueue[ListenerSetReports]()
	routeReportQueue := utils.NewAsyncQueue[RouteReports]()

	// Register to separate singleton collections instead of a single merged report
	s.gatewayReports.Register(func(o krt.Event[GatewayReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		gatewayReportQueue.Enqueue(o.Latest())
	})

	s.listenerSetReports.Register(func(o krt.Event[ListenerSetReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		listenerSetReportQueue.Enqueue(o.Latest())
	})

	s.routeReports.Register(func(o krt.Event[RouteReports]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection
			return
		}
		routeReportQueue.Enqueue(o.Latest())
	})

	// Start separate goroutines for each status syncer
	routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
	listenerSetStatusLogger := logger.With("subcomponent", "listenerSetStatusSyncer")
	gatewayStatusLogger := logger.With("subcomponent", "gatewayStatusSyncer")

	// Gateway status syncer
	go func() {
		for {
			gatewayReports, err := gatewayReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue gateway reports", "error", err)
				return
			}
			s.syncGatewayStatus(ctx, gatewayStatusLogger, gatewayReports)
		}
	}()

	// Listener set status syncer
	go func() {
		for {
			listenerSetReports, err := listenerSetReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue listener set reports", "error", err)
				return
			}
			s.syncListenerSetStatus(ctx, listenerSetStatusLogger, listenerSetReports)
		}
	}()

	// Route status syncer
	go func() {
		for {
			routeReports, err := routeReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue route reports", "error", err)
				return
			}
			s.syncRouteStatus(ctx, routeStatusLogger, routeReports)
		}
	}()

	s.xDS.RegisterBatch(func(events []krt.Event[agentGwXdsResources]) {
		for _, e := range events {
			snap := e.Latest()
			if e.Event == controllers.EventDelete {
				// TODO: we should probably clear, but this has been causing some undiagnosed issues.
				//s.xdsCache.ClearSnapshot(snap.ResourceName())
				continue
			}
			snapshot := &agentGwSnapshot{
				Resources: snap.ResourceConfig,
				Addresses: snap.AddressConfig,
			}
			logger.Debug("setting xds snapshot", "resource_name", snap.ResourceName())
			logger.Debug("snapshot config", "resource_snapshot", snapshot.Resources, "workload_snapshot", snapshot.Addresses)
			err := s.xdsCache.SetSnapshot(ctx, snap.ResourceName(), snapshot)
			if err != nil {
				logger.Error("failed to set xds snapshot", "resource_name", snap.ResourceName(), "error", err.Error())
				continue
			}
		}
	}, true)

	s.ready.Store(true)
	<-ctx.Done()
	return nil
}

func (s *AgentGwSyncer) HasSynced() bool {
	return s.ready.Load()
}

type agentGwSnapshot struct {
	Resources  envoycache.Resources
	Addresses  envoycache.Resources
	VersionMap map[string]map[string]string
}

func (m *agentGwSnapshot) GetResources(typeURL string) map[string]envoytypes.Resource {
	resources := m.GetResourcesAndTTL(typeURL)
	result := make(map[string]envoytypes.Resource, len(resources))
	for k, v := range resources {
		result[k] = v.Resource
	}
	return result
}

func (m *agentGwSnapshot) GetResourcesAndTTL(typeURL string) map[string]envoytypes.ResourceWithTTL {
	switch typeURL {
	case TargetTypeResourceUrl:
		return m.Resources.Items
	case TargetTypeAddressUrl:
		return m.Addresses.Items
	default:
		return nil
	}
}

func (m *agentGwSnapshot) GetVersion(typeURL string) string {
	switch typeURL {
	case TargetTypeResourceUrl:
		return m.Resources.Version
	case TargetTypeAddressUrl:
		return m.Addresses.Version
	default:
		return ""
	}
}

func (m *agentGwSnapshot) ConstructVersionMap() error {
	if m == nil {
		return fmt.Errorf("missing snapshot")
	}
	if m.VersionMap != nil {
		return nil
	}

	m.VersionMap = make(map[string]map[string]string)
	resources := map[string]map[string]envoytypes.ResourceWithTTL{
		TargetTypeResourceUrl: m.Resources.Items,
		TargetTypeAddressUrl:  m.Addresses.Items,
	}

	for typeUrl, items := range resources {
		inner := make(map[string]string, len(items))
		for _, r := range items {
			marshaled, err := envoycache.MarshalResource(r.Resource)
			if err != nil {
				return err
			}
			v := envoycache.HashResource(marshaled)
			if v == "" {
				return fmt.Errorf("failed to build resource version")
			}
			inner[envoycache.GetResourceName(r.Resource)] = v
		}
		m.VersionMap[typeUrl] = inner
	}
	return nil
}

func (m *agentGwSnapshot) GetVersionMap(typeURL string) map[string]string {
	return m.VersionMap[typeURL]
}

var _ envoycache.ResourceSnapshot = &agentGwSnapshot{}

func (s *AgentGwSyncer) syncRouteStatus(ctx context.Context, logger *slog.Logger, routeReports RouteReports) {
	stopwatch := utils.NewTranslatorStopWatch("RouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	// TODO: add routeStatusMetrics

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
		getRouteFunc func() client.Object,
		statusUpdater func(route client.Object) error,
	) error {
		return retry.Do(
			func() error {
				route := getRouteFunc()
				err := s.mgr.GetClient().Get(ctx, routeKey, route)
				if err != nil {
					if apierrors.IsNotFound(err) {
						// the route is not found, we can't report status on it
						// if it's recreated, we'll retranslate it anyway
						return nil
					}
					logger.Error("error getting route", logKeyError, err, logKeyResourceRef, routeKey, logKeyRouteType, routeType)
					return err
				}
				if err := statusUpdater(route); err != nil {
					logger.Debug("error updating status for route", logKeyError, err, logKeyResourceRef, routeKey, logKeyRouteType, routeType)
					return err
				}
				return nil
			},
			retry.Attempts(maxRetryAttempts),
			retry.Delay(retryDelay),
			retry.DelayType(retry.BackOffDelay),
		)
	}

	// Create a minimal ReportMap with just the route reports for BuildRouteStatus to work
	rm := reports.ReportMap{
		HTTPRoutes: routeReports.HTTPRoutes,
		GRPCRoutes: routeReports.GRPCRoutes,
		TCPRoutes:  routeReports.TCPRoutes,
		TLSRoutes:  routeReports.TLSRoutes,
	}

	// Helper function to build route status and update if needed
	buildAndUpdateStatus := func(route client.Object, routeType string) error {
		var status *gwv1.RouteStatus
		switch r := route.(type) {
		case *gwv1.HTTPRoute: // TODO: beta1?
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1alpha2.TCPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1alpha2.TLSRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1.GRPCRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		default:
			logger.Warn("unsupported route type", logKeyRouteType, routeType, logKeyResourceRef, client.ObjectKeyFromObject(route))
			return nil
		}

		// Update the status
		return s.mgr.GetClient().Status().Update(ctx, route)
	}

	for rnn := range routeReports.HTTPRoutes {
		err := syncStatusWithRetry(
			wellknown.HTTPRouteKind,
			rnn,
			func() client.Object {
				return new(gwv1.HTTPRoute)
			},
			func(route client.Object) error {
				return buildAndUpdateStatus(route, wellknown.HTTPRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating HTTPRoute status", logKeyError, err, "route", rnn)
		}
	}
}

// syncGatewayStatus will build and update status for all Gateways in gateway reports
func (s *AgentGwSyncer) syncGatewayStatus(ctx context.Context, logger *slog.Logger, gatewayReports GatewayReports) {
	stopwatch := utils.NewTranslatorStopWatch("GatewayStatusSyncer")
	stopwatch.Start()

	// TODO: add gatewayStatusMetrics

	// Create a minimal ReportMap with just the gateway reports for BuildGWStatus to work
	rm := reports.ReportMap{
		Gateways: gatewayReports.Reports,
	}

	// TODO: retry within loop per GW rather that as a full block
	err := retry.Do(func() error {
		for gwnn := range gatewayReports.Reports {
			gw := gwv1.Gateway{}
			err := s.mgr.GetClient().Get(ctx, gwnn, &gw)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// the gateway is not found, we can't report status on it
					// if it's recreated, we'll retranslate it anyway
					continue
				}
				logger.Info("error getting gw", logKeyError, err, logKeyGateway, gwnn.String())
				return err
			}

			// Only process agentgateway classes - others are handled by ProxySyncer
			if string(gw.Spec.GatewayClassName) != s.agentGatewayClassName {
				logger.Debug("skipping status sync for non-agentgateway", logKeyGateway, gwnn.String())
				continue
			}

			gwStatusWithoutAddress := gw.Status
			gwStatusWithoutAddress.Addresses = nil
			if status := rm.BuildGWStatus(ctx, gw); status != nil {
				if !isGatewayStatusEqual(&gwStatusWithoutAddress, status) {
					gw.Status = *status
					if err := s.mgr.GetClient().Status().Patch(ctx, &gw, client.Merge); err != nil {
						logger.Error("error patching gateway status", logKeyError, err, logKeyGateway, gwnn.String())
						return err
					}
					logger.Info("patched gw status", logKeyGateway, gwnn.String())
				} else {
					logger.Info("skipping k8s gateway status update, status equal", logKeyGateway, gwnn.String())
				}
			}
		}
		return nil
	},
		retry.Attempts(maxRetryAttempts),
		retry.Delay(retryDelay),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating gateway statuses", logKeyError, err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debug("synced gw status for gateways", "count", len(gatewayReports.Reports), "duration", duration)
}

// syncListenerSetStatus will build and update status for all Listener Sets in listener set reports
func (s *AgentGwSyncer) syncListenerSetStatus(ctx context.Context, logger *slog.Logger, listenerSetReports ListenerSetReports) {
	stopwatch := utils.NewTranslatorStopWatch("ListenerSetStatusSyncer")
	stopwatch.Start()

	// TODO: add listenerStatusMetrics

	// Create a minimal ReportMap with just the listener set reports for BuildListenerSetStatus to work
	rm := reports.ReportMap{
		ListenerSets: listenerSetReports.Reports,
	}

	// TODO: retry within loop per LS rathen that as a full block
	err := retry.Do(func() error {
		for lsnn := range listenerSetReports.Reports {
			ls := gwxv1a1.XListenerSet{}
			err := s.mgr.GetClient().Get(ctx, lsnn, &ls)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// the listener set is not found, we can't report status on it
					// if it's recreated, we'll retranslate it anyway
					continue
				}
				logger.Info("error getting ls", "error", err.Error())
				return err
			}
			lsStatus := ls.Status
			if status := rm.BuildListenerSetStatus(ctx, ls); status != nil {
				if !isListenerSetStatusEqual(&lsStatus, status) {
					ls.Status = *status
					if err := s.mgr.GetClient().Status().Patch(ctx, &ls, client.Merge); err != nil {
						logger.Error("error patching listener set status", logKeyError, err, logKeyGateway, lsnn.String())
						return err
					}
					logger.Info("patched ls status", "listenerset", lsnn.String())
				} else {
					logger.Info("skipping k8s ls status update, status equal", "listenerset", lsnn.String())
				}
			}
		}
		return nil
	},
		retry.Attempts(maxRetryAttempts),
		retry.Delay(retryDelay),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating listener set statuses", logKeyError, err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debug("synced listener sets status for listener set", "count", len(listenerSetReports.Reports), "duration", duration.String())
}

// TODO: refactor proxy_syncer status syncing to use the same logic as agentgateway syncer

var opts = cmp.Options{
	cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
		return k == "lastTransitionTime"
	}),
}

// isRouteStatusEqual compares two RouteStatus objects directly
func isRouteStatusEqual(objA, objB *gwv1.RouteStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

func isListenerSetStatusEqual(objA, objB *gwxv1a1.ListenerSetStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

func isGatewayStatusEqual(objA, objB *gwv1.GatewayStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

// mergeRouteReports is a helper function to merge route reports
func mergeRouteReports(merged map[types.NamespacedName]*reports.RouteReport, source map[types.NamespacedName]*reports.RouteReport) {
	for rnn, rr := range source {
		// if we haven't encountered this route, just copy it over completely
		old := merged[rnn]
		if old == nil {
			merged[rnn] = rr
			continue
		}
		// else, this route has already been seen for a proxy, merge this proxy's parents
		// into the merged report
		maps.Copy(merged[rnn].Parents, rr.Parents)
	}
}

package agentgatewaysyncer

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	agentgateway "github.com/agentgateway/agentgateway/go/api"
	"github.com/agentgateway/agentgateway/go/api/a2a"
	"github.com/agentgateway/agentgateway/go/api/mcp"
	envoytypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

var logger = logging.New("agentgateway/syncer")

// AgentGwSyncer synchronizes Kubernetes Gateway API resources with xDS for agentgateway proxies.
// It watches Gateway resources with the agentgateway class and translates them to agentgateway configuration.
type AgentGwSyncer struct {
	commonCols     *common.CommonCollections
	controllerName string
	xDS            krt.Collection[agentGwXdsResources]
	xdsCache       envoycache.SnapshotCache
	istioClient    kube.Client

	waitForSync []cache.InformerSynced
}

func NewAgentGwSyncer(
	ctx context.Context,
	controllerName string,
	mgr manager.Manager,
	client kube.Client,
	commonCols *common.CommonCollections,
	xdsCache envoycache.SnapshotCache,
) *AgentGwSyncer {
	// TODO: register types (auth, policy, etc.) if necessary
	return &AgentGwSyncer{
		commonCols:     commonCols,
		controllerName: controllerName,
		xdsCache:       xdsCache,
		// mgr:            mgr,
		istioClient: client,
	}
}

type agentGwXdsResources struct {
	types.NamespacedName

	reports            reports.ReportMap
	AgentGwA2AServices envoycache.Resources
	AgentGwMcpServices envoycache.Resources
	Listeners          envoycache.Resources
}

func (r agentGwXdsResources) ResourceName() string {
	return xds.OwnerNamespaceNameID(OwnerNodeId, r.Namespace, r.Name)
}

func (r agentGwXdsResources) Equals(in agentGwXdsResources) bool {
	return r.NamespacedName == in.NamespacedName &&
		report{r.reports}.Equals(report{in.reports}) &&
		r.AgentGwA2AServices.Version == in.AgentGwA2AServices.Version &&
		r.AgentGwMcpServices.Version == in.AgentGwMcpServices.Version
}

type envoyResourceWithName struct {
	inner   envoytypes.ResourceWithName
	version uint64
}

func (r envoyResourceWithName) ResourceName() string {
	return r.inner.GetName()
}

func (r envoyResourceWithName) Equals(in envoyResourceWithName) bool {
	return r.version == in.version
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

type agentGwService struct {
	krt.Named
	ip       string
	port     int
	path     string
	protocol string // currently only A2A and MCP
	// The listeners which are allowed to connect to the target.
	allowedListeners []string
}

func (r agentGwService) Equals(in agentGwService) bool {
	return r.ip == in.ip && r.port == in.port && r.path == in.path && r.protocol == in.protocol && slices.Equal(r.allowedListeners, in.allowedListeners)
}

type report struct {
	// lower case so krt doesn't error in debug handler
	reportMap reports.ReportMap
}

func (r report) ResourceName() string {
	return "report"
}

func (r report) Equals(in report) bool {
	return maps.Equal(r.reportMap.Gateways, in.reportMap.Gateways) &&
		maps.Equal(r.reportMap.HTTPRoutes, in.reportMap.HTTPRoutes) &&
		maps.Equal(r.reportMap.TCPRoutes, in.reportMap.TCPRoutes)
}

func (s *AgentGwSyncer) Init(krtopts krtutil.KrtOptions) {
	logger.Debug("init agentgateway Syncer", "controllername", s.controllerName)

	// TODO: convert auth to rbac json config for agentgateways

	gatewaysCol := krt.NewCollection(s.commonCols.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *ir.Gateway {
		if gw.Obj.Spec.GatewayClassName != wellknown.AgentGatewayClassName {
			return nil
		}
		return &gw
	}, krtopts.ToOptions("agentgateway")...)

	// TODO(npolshak): optimize this in the future with an index
	agentGwServices := krt.NewManyCollection(s.commonCols.Services, func(kctx krt.HandlerContext, s *corev1.Service) []agentGwService {
		var allowedA2AListeners, allowedMCPListeners []string

		gws := krt.Fetch(kctx, gatewaysCol)
		for _, gw := range gws {
			for _, listener := range gw.Listeners {
				if listener.Protocol != A2AProtocol && listener.Protocol != MCPProtocol {
					continue
				}
				logger.Debug("found agentgateway service", "namespace", s.Namespace, "name", s.Name)
				if listener.AllowedRoutes == nil {
					// only allow agent services in same namespace
					if s.Namespace == gw.Obj.Namespace {
						if listener.Protocol == A2AProtocol {
							allowedA2AListeners = append(allowedA2AListeners, string(listener.Name))
						} else {
							allowedMCPListeners = append(allowedMCPListeners, string(listener.Name))
						}
					}
				} else if listener.AllowedRoutes.Namespaces.From != nil {
					switch *listener.AllowedRoutes.Namespaces.From {
					case gwv1.NamespacesFromAll:
						if listener.Protocol == A2AProtocol {
							allowedA2AListeners = append(allowedA2AListeners, string(listener.Name))
						} else {
							allowedMCPListeners = append(allowedMCPListeners, string(listener.Name))
						}
					case gwv1.NamespacesFromSame:
						// only allow agent services in same namespace
						if s.Namespace == gw.Obj.Namespace {
							if listener.Protocol == A2AProtocol {
								allowedA2AListeners = append(allowedA2AListeners, string(listener.Name))
							} else {
								allowedMCPListeners = append(allowedMCPListeners, string(listener.Name))
							}
						}
					case gwv1.NamespacesFromSelector:
						// TODO: implement namespace selectors with gateway index
						logger.Error("namespace selectors not supported for agentgateways")
						continue
					}
				}
			}
		}
		return translateAgentService(s, allowedA2AListeners, allowedMCPListeners)
	})
	xdsA2AServices := krt.NewCollection(agentGwServices, func(kctx krt.HandlerContext, s agentGwService) *envoyResourceWithName {
		if s.protocol != A2AProtocol {
			return nil
		}
		t := &a2a.Target{
			Name:      getTargetName(s.ResourceName()),
			Host:      s.ip,
			Port:      uint32(s.port),
			Path:      s.path,
			Listeners: s.allowedListeners,
		}
		return &envoyResourceWithName{inner: t, version: utils.HashProto(t)}
	}, krtopts.ToOptions("a2a-target-xds")...)
	xdsMcpServices := krt.NewCollection(agentGwServices, func(kctx krt.HandlerContext, s agentGwService) *envoyResourceWithName {
		if s.protocol != MCPProtocol {
			return nil
		}
		t := &mcp.Target{
			// Note: No slashes allowed here (must match ^[a-zA-Z0-9-]+$)
			Name: getTargetName(s.ResourceName()),
			Target: &mcp.Target_Sse{
				Sse: &mcp.Target_SseTarget{
					Host: s.ip,
					Port: uint32(s.port),
					Path: s.path,
				},
			},
			Listeners: s.allowedListeners,
		}
		return &envoyResourceWithName{inner: t, version: utils.HashProto(t)}
	}, krtopts.ToOptions("mcp-target-xds")...)

	// translate gateways to xds
	s.xDS = krt.NewCollection(gatewaysCol, func(kctx krt.HandlerContext, gw ir.Gateway) *agentGwXdsResources {
		// listeners for the agentgateway
		agwListeners := make([]envoytypes.Resource, 0, len(gw.Listeners))
		var listenerVersion uint64
		var listener *agentgateway.Listener
		for _, gwListener := range gw.Listeners {
			var protocol agentgateway.Listener_Protocol
			switch string(gwListener.Protocol) {
			case MCPProtocol:
				protocol = agentgateway.Listener_MCP
			case A2AProtocol:
				protocol = agentgateway.Listener_A2A
			default:
				// Not a valid protocol for agentgateway
				continue
			}

			listener = &agentgateway.Listener{
				Name:     string(gwListener.Name),
				Protocol: protocol,
				// TODO: Add support for stdio listener
				Listener: &agentgateway.Listener_Sse{
					Sse: &agentgateway.SseListener{
						Address: "[::]",
						Port:    uint32(gwListener.Port),
					},
				},
			}

			// Update listenerVersion to be the result
			listenerVersion ^= utils.HashProto(listener)
			agwListeners = append(agwListeners, listener)
		}

		// a2a services
		a2aServiceResources := krt.Fetch(kctx, xdsA2AServices)
		logger.Debug("found A2A resources for gateway", "total_services", len(a2aServiceResources), "resource_ref", gw.ResourceName())
		a2aResources := make([]envoytypes.Resource, len(a2aServiceResources))
		var a2aVersion uint64
		for i, res := range a2aServiceResources {
			a2aVersion ^= res.version
			target := res.inner.(*a2a.Target)
			a2aResources[i] = target
		}
		// mcp services
		mcpServiceResources := krt.Fetch(kctx, xdsMcpServices)
		logger.Debug("found MCP resources for gateway", "total_services", len(mcpServiceResources), "resource_ref", gw.ResourceName())
		mcpResources := make([]envoytypes.Resource, len(mcpServiceResources))
		var mcpVersion uint64
		for i, res := range mcpServiceResources {
			mcpVersion ^= res.version
			target := res.inner.(*mcp.Target)
			mcpResources[i] = target
		}
		result := &agentGwXdsResources{
			NamespacedName:     types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name},
			AgentGwA2AServices: envoycache.NewResources(fmt.Sprintf("%d", a2aVersion), a2aResources),
			AgentGwMcpServices: envoycache.NewResources(fmt.Sprintf("%d", mcpVersion), mcpResources),
			Listeners:          envoycache.NewResources(fmt.Sprintf("%d", listenerVersion), agwListeners),
		}
		logger.Debug("created XDS resources for with ID", "gwname", gw.Name, "resourceid", result.ResourceName())
		return result
	}, krtopts.ToOptions("agentgateway-xds")...)

	s.waitForSync = []cache.InformerSynced{
		s.commonCols.HasSynced,
		xdsA2AServices.HasSynced,
		xdsMcpServices.HasSynced,
		gatewaysCol.HasSynced,
		agentGwServices.HasSynced,
		s.xDS.HasSynced,
	}
}

func (s *AgentGwSyncer) Start(ctx context.Context) error {
	logger.Info("starting agentgateway Syncer", "controllername", s.controllerName)
	logger.Info("waiting for agentgateway cache to sync")

	// Wait for cache to sync
	if !kube.WaitForCacheSync("agentgateway syncer", ctx.Done(), s.waitForSync...) {
		return fmt.Errorf("agentgateway syncer waiting for cache to sync failed")
	}

	s.xDS.RegisterBatch(func(events []krt.Event[agentGwXdsResources], _ bool) {
		for _, e := range events {
			r := e.Latest()
			if e.Event == controllers.EventDelete {
				s.xdsCache.ClearSnapshot(r.ResourceName())
				continue
			}
			snapshot := &agentGwSnapshot{
				AgentGwA2AServices: r.AgentGwA2AServices,
				AgentGwMcpServices: r.AgentGwMcpServices,
				Listeners:          r.Listeners,
			}
			logger.Debug("setting xds snapshot", "resourcename", r.ResourceName())
			err := s.xdsCache.SetSnapshot(ctx, r.ResourceName(), snapshot)
			if err != nil {
				logger.Error("failed to set xds snapshot", "resourcename", r.ResourceName(), "error", err.Error())
				continue
			}
		}
	}, true)

	return nil
}

type agentGwSnapshot struct {
	AgentGwA2AServices envoycache.Resources
	AgentGwMcpServices envoycache.Resources
	Listeners          envoycache.Resources
	VersionMap         map[string]map[string]string
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
	case TargetTypeA2AUrl:
		return m.AgentGwA2AServices.Items
	case TargetTypeMcpUrl:
		return m.AgentGwMcpServices.Items
	case TargetTypeListenerUrl:
		return m.Listeners.Items
	default:
		return nil
	}
}

func (m *agentGwSnapshot) GetVersion(typeURL string) string {
	switch typeURL {
	case TargetTypeA2AUrl:
		return m.AgentGwA2AServices.Version
	case TargetTypeMcpUrl:
		return m.AgentGwMcpServices.Version
	case TargetTypeListenerUrl:
		return m.Listeners.Version
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
		TargetTypeA2AUrl:      m.AgentGwA2AServices.Items,
		TargetTypeMcpUrl:      m.AgentGwMcpServices.Items,
		TargetTypeListenerUrl: m.Listeners.Items,
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

// getTargetName sanitizes the given resource name to ensure it matches the AgentGateway required pattern:
// ^[a-zA-Z0-9-]+$ by replacing slashes and removing invalid characters.
func getTargetName(resourceName string) string {
	var (
		invalidCharsRegex      = regexp.MustCompile(`[^a-zA-Z0-9-]+`)
		consecutiveDashesRegex = regexp.MustCompile(`-+`)
	)

	// Replace all invalid characters with dashes
	sanitized := invalidCharsRegex.ReplaceAllString(resourceName, "-")

	// Remove leading/trailing dashes and collapse consecutive dashes
	sanitized = strings.Trim(sanitized, "-")
	sanitized = consecutiveDashesRegex.ReplaceAllString(sanitized, "-")

	return sanitized
}

func translateAgentService(svc *corev1.Service, allowedA2AListeners, allowedMCPListeners []string) []agentGwService {
	var svcs []agentGwService

	if svc.Spec.ClusterIP == "" && svc.Spec.ExternalName == "" {
		// Return early if there's no valid IP or external name set on the service
		return svcs
	}

	addr := svc.Spec.ClusterIP
	if addr == "" {
		addr = svc.Spec.ExternalName
	}

	for _, port := range svc.Spec.Ports {
		if port.AppProtocol == nil {
			continue
		}
		appProtocol := *port.AppProtocol
		var path string
		var allowedListeners []string

		switch appProtocol {
		case A2AProtocol:
			path = svc.Annotations[A2APathAnnotation]
			allowedListeners = allowedA2AListeners
		case MCPProtocol:
			path = svc.Annotations[MCPPathAnnotation]
			allowedListeners = allowedMCPListeners
		default:
			// Skip unsupported protocols
			continue
		}

		svcs = append(svcs, agentGwService{
			Named: krt.Named{
				Name:      svc.Name,
				Namespace: svc.Namespace,
			},
			ip:               addr,
			port:             int(port.Port),
			path:             path,
			protocol:         appProtocol,
			allowedListeners: allowedListeners,
		})
	}
	return svcs
}

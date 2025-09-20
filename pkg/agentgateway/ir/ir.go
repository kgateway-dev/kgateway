package ir

import (
	"fmt"
	"maps"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("agentgateway")

type AgwResourcesForGateway struct {
	// agent gateway dataplane resources
	Resources []*api.Resource
	// gateway name
	Gateway types.NamespacedName
	// status for the gateway
	Report reports.ReportMap
	// track which routes are attached to the gateway listener for each resource type (HTTPRoute, TCPRoute, etc)
	AttachedRoutes map[string]uint
}

func (g AgwResourcesForGateway) ResourceName() string {
	// need a unique name per resource
	return g.Gateway.String() + getResourceListName(g.Resources)
}

func getResourceListName(resources []*api.Resource) string {
	names := make([]string, len(resources))
	for i, res := range resources {
		names[i] = GetAgwResourceName(res)
	}
	return strings.Join(names, ",")
}

func GetAgwResourceName(r *api.Resource) string {
	switch t := r.GetKind().(type) {
	case *api.Resource_Bind:
		return "bind/" + t.Bind.GetKey()
	case *api.Resource_Listener:
		return "listener/" + t.Listener.GetKey()
	case *api.Resource_Backend:
		return "backend/" + t.Backend.GetName()
	case *api.Resource_Route:
		return "route/" + t.Route.GetKey()
	case *api.Resource_Policy:
		return "policy/" + t.Policy.GetName()
	default:
		logger.Error("unknown Agw resource", "type", fmt.Sprintf("%T", t))
		return "unknown/" + r.String()
	}
}

func (g AgwResourcesForGateway) Equals(other AgwResourcesForGateway) bool {
	// Don't compare reports, as they are not part of the AgwResource equality and synced separately
	for i := range g.Resources {
		if !proto.Equal(g.Resources[i], other.Resources[i]) {
			return false
		}
	}
	if !maps.Equal(g.AttachedRoutes, other.AttachedRoutes) {
		return false
	}
	return g.Gateway == other.Gateway
}

type AgwCacheAddress struct {
	NamespacedName types.NamespacedName
	ResourceNames  string

	Address             proto.Message
	AddressResourceName string
	AddressVersion      uint64

	VersionMap map[string]map[string]string
}

func (r AgwCacheAddress) ResourceName() string {
	return r.ResourceNames
}

func (r AgwCacheAddress) Equals(in AgwCacheAddress) bool {
	return r.NamespacedName.Name == in.NamespacedName.Name && r.NamespacedName.Namespace == in.NamespacedName.Namespace &&
		proto.Equal(r.Address, in.Address) &&
		r.AddressVersion == in.AddressVersion &&
		r.AddressResourceName == in.AddressResourceName
}

package serviceentry

import (
	"context"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	BackendClusterPrefix = "istio-se"
)

var logger = logging.New("plugin/serviceentry")

type Aliaser = func(se *networkingclient.ServiceEntry) []ir.ObjectSource

func HostnameAliaser(se *networkingclient.ServiceEntry) []ir.ObjectSource {
	return slices.Map(se.Spec.GetHosts(), func(hostname string) ir.ObjectSource {
		return ir.ObjectSource{
			Group:     wellknown.HostnameGVK.Group,
			Kind:      wellknown.HostnameGVK.Kind,
			Name:      hostname,
			Namespace: "", // global
		}
	})
}

// PortMapper is a function that returns the port to be used for the endpoint.
// It can be customized to support different port mapping strategies.
type PortMapper = func(name string, portMapping map[string]uint32, defalutValue int32, labels map[string]string) int32

// DefaultPortMapper will respect mapped ports if they are specified in the portMapping map.
// If no target port is specified, it will return the default value.
func DefaultPortMapper(name string, portMapping map[string]uint32, defalutValue int32, labels map[string]string) int32 {
	if portMapping == nil {
		return defalutValue
	}
	if override := portMapping[name]; override > 0 {
		return int32(override)
	}
	return defalutValue
}

type Options struct {
	Aliaser
	PortMapper
}

func NewPlugin(
	ctx context.Context,
	commonCols *common.CommonCollections,
) extensionsplug.Plugin {
	return NewPluginWithOpts(ctx, commonCols, Options{
		Aliaser:    HostnameAliaser,
		PortMapper: DefaultPortMapper,
	})
}

func NewPluginWithOpts(
	_ context.Context,
	commonCols *common.CommonCollections,
	opts Options,
) extensionsplug.Plugin {
	seCollections := initServiceEntryCollections(commonCols, opts)
	return extensionsplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extensionsplug.BackendPlugin{
			wellknown.ServiceEntryGVK.GroupKind(): {
				BackendInit: ir.BackendInit{
					InitBackend: seCollections.initServiceEntryBackend,
				},
				Backends: seCollections.Backends,

				AliasKinds: []schema.GroupKind{
					// allow backendRef with networking.istio.io/Hostname
					wellknown.HostnameGVK.GroupKind(),
					// alias to ourself because one SE -> multiple Backends
					wellknown.ServiceEntryGVK.GroupKind(),
				},

				Endpoints: seCollections.Endpoints,
			},
		},
		ExtraHasSynced: func() bool {
			return seCollections.HasSynced()
		},
	}
}

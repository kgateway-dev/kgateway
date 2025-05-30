package serviceentry

import (
	"context"

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

func NewPlugin(
	_ context.Context,
	commonCols *common.CommonCollections,
) extensionsplug.Plugin {
	seCollections := initServiceEntryCollections(commonCols)
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

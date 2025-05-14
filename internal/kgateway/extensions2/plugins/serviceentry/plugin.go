package serviceentry

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	BackendClusterPrefix = "istio-se"
)

func NewPlugin(
	ctx context.Context,
	commonCols *common.CommonCollections,
) extensionsplug.Plugin {
	seCollections := initServiceEntryCollections(ctx, commonCols)
	return extensionsplug.Plugin{
		ContributesPolicies: extensionsplug.ContributesPolicies{
			wellknown.ServiceEntryGVK.GroupKind(): extensionsplug.PolicyPlugin{
				GetBackendForRef: seCollections.getBackendForRef,
			},
		},
		ContributesBackends: map[schema.GroupKind]extensionsplug.BackendPlugin{
			wellknown.ServiceEntryGVK.GroupKind(): {
				BackendInit: ir.BackendInit{
					InitBackend: seCollections.initServiceEntryBackend,
				},
				Backends:  seCollections.Backends,
				Endpoints: seCollections.Endpoints,
			},
		},
		ExtraHasSynced: func() bool {
			return seCollections.HasSynced()
		},
	}
}

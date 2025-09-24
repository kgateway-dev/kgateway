package serviceentry

import (
	"context"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"

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

type Options struct {
	Aliaser
}

func NewPlugin(
	ctx context.Context,
	commonCols *collections.CommonCollections,
) sdk.Plugin {
	return NewPluginWithOpts(ctx, commonCols, Options{
		Aliaser: HostnameAliaser,
	})
}

func NewPluginWithOpts(
	_ context.Context,
	commonCols *collections.CommonCollections,
	opts Options,
) sdk.Plugin {
	seCollections := initServiceEntryCollections(commonCols, opts)
	return sdk.Plugin{
		ContributesBackends: map[schema.GroupKind]sdk.BackendPlugin{
			wellknown.ServiceEntryGVK.GroupKind(): {
				BackendInit: ir.BackendInit{
					InitEnvoyBackend: seCollections.initServiceEntryBackend,
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

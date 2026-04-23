package serviceentry

import (
	"context"
	"strings"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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
	// WorkloadEntriesExclusionLabelKeys is the set of label keys that, if present on a WorkloadEntry,
	// cause it to be excluded from endpoint discovery. When nil, the default from
	// Settings.WorkloadEntriesExclusionLabels is used. Use an empty set to explicitly disable
	// exclusions.
	WorkloadEntriesExclusionLabelKeys sets.Set[string]
}

func NewPlugin(
	ctx context.Context,
	commonCols *collections.CommonCollections,
) sdk.Plugin {
	if !commonCols.Settings.EnableIstioIntegration {
		return sdk.Plugin{}
	}
	return NewPluginWithOpts(ctx, commonCols, Options{
		Aliaser: HostnameAliaser,
	})
}

// ParseExclusionLabels splits a comma-separated string of label keys into a set.
func ParseExclusionLabels(raw string) sets.Set[string] {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		if k := strings.TrimSpace(p); k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return sets.New(keys...)
}

func NewPluginWithOpts(
	_ context.Context,
	commonCols *collections.CommonCollections,
	opts Options,
) sdk.Plugin {
	// If the caller didn't supply an explicit exclusion set, fall back to the value
	// from settings (KGW_WORKLOAD_ENTRIES_EXCLUSION_LABELS env var).
	if opts.WorkloadEntriesExclusionLabelKeys == nil {
		opts.WorkloadEntriesExclusionLabelKeys = ParseExclusionLabels(commonCols.Settings.WorkloadEntriesExclusionLabels)
	}
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
					// alias to ourselves because one SE -> multiple Backends
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

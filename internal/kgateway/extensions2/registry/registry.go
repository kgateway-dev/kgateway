package registry

import (
	"context"
	"maps"

	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/backend"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/backendtlspolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/destrule"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/directresponse"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/httplistenerpolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/istio"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/kubernetes"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/sandwich"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/serviceentry"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/trafficpolicy"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

func mergedGw(funcs []sdk.GwTranslatorFactory) sdk.GwTranslatorFactory {
	return func(gw *gwv1.Gateway) sdk.KGwTranslator {
		for _, f := range funcs {
			ret := f(gw)
			if ret != nil {
				return ret
			}
		}
		return nil
	}
}

func mergeSynced(funcs []func() bool) func() bool {
	return func() bool {
		for _, f := range funcs {
			if !f() {
				return false
			}
		}
		return true
	}
}

func MergePlugins(plug ...sdk.Plugin) sdk.Plugin {
	ret := sdk.Plugin{
		ContributesPolicies:     make(map[schema.GroupKind]sdk.PolicyPlugin),
		ContributesBackends:     make(map[schema.GroupKind]sdk.BackendPlugin),
		ContributesRegistration: make(map[schema.GroupKind]func()),
	}
	var funcs []sdk.GwTranslatorFactory
	var hasSynced []func() bool
	for _, p := range plug {
		maps.Copy(ret.ContributesPolicies, p.ContributesPolicies)
		maps.Copy(ret.ContributesBackends, p.ContributesBackends)
		maps.Copy(ret.ContributesRegistration, p.ContributesRegistration)
		if p.ContributesGwTranslator != nil {
			funcs = append(funcs, p.ContributesGwTranslator)
		}
		if p.ExtraHasSynced != nil {
			hasSynced = append(hasSynced, p.ExtraHasSynced)
		}
	}
	ret.ContributesGwTranslator = mergedGw(funcs)
	ret.ExtraHasSynced = mergeSynced(hasSynced)
	return ret
}

func Plugins(ctx context.Context, commoncol *common.CommonCollections) []sdk.Plugin {
	return []sdk.Plugin{
		// Add plugins here
		backend.NewPlugin(ctx, commoncol),
		trafficpolicy.NewPlugin(ctx, commoncol),
		directresponse.NewPlugin(ctx, commoncol),
		kubernetes.NewPlugin(ctx, commoncol),
		istio.NewPlugin(ctx, commoncol),
		destrule.NewPlugin(ctx, commoncol),
		httplistenerpolicy.NewPlugin(ctx, commoncol),
		backendtlspolicy.NewPlugin(ctx, commoncol),
		serviceentry.NewPlugin(ctx, commoncol),
		waypoint.NewPlugin(ctx, commoncol),
		sandwich.NewPlugin(),
	}
}

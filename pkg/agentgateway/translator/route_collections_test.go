package translator_test

import (
	"context"
	"testing"

	"istio.io/istio/pkg/slices"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/status"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/testutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestRouteCollection(t *testing.T) {
	testutils.RunForDirectory(t, "testdata/routes", func(t *testing.T, ctx plugins.PolicyCtx) (*gwv1.PolicyStatus, []ir.AgwResource) {
		ri := testutils.RouteInputs(ctx)
		agwRoutes, _ := translator.AgwRouteCollection(&status.StatusCollections{},
			ctx.Collections.HTTPRoutes,
			ctx.Collections.GRPCRoutes,
			ctx.Collections.TCPRoutes,
			ctx.Collections.TLSRoutes,
			ri,
			krtutil.KrtOptions{})
		agwRoutes.WaitUntilSynced(context.Background().Done())
		return nil, slices.SortBy(agwRoutes.List(), func(a ir.AgwResource) string {
			return a.ResourceName()
		})
	})
}

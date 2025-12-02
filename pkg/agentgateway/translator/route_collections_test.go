package translator_test

import (
	"strings"
	"testing"

	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/testutils"
)

func TestRouteCollection(t *testing.T) {
	testutils.RunForDirectory(t, "testdata/routes", func(t *testing.T, ctx plugins.PolicyCtx) (any, []ir.AgwResource) {
		sq, ri := testutils.Syncer(t, ctx, "HTTPRoute", "GRPCRoute")
		r := ri.Outputs.Resources.List()
		r = slices.FilterInPlace(r, func(resource ir.AgwResource) bool {
			x := ir.GetAgwResourceName(resource.Resource)
			return strings.HasPrefix(x, "route/")
		})
		return sq.Dump(), slices.SortBy(r, func(a ir.AgwResource) string {
			return a.ResourceName()
		})
	})
}

func TestGatewayCollection(t *testing.T) {
	testutils.RunForDirectory(t, "testdata/gateways", func(t *testing.T, ctx plugins.PolicyCtx) (any, []ir.AgwResource) {
		sq, ri := testutils.Syncer(t, ctx, "Gateway")
		r := ri.Outputs.Resources.List()
		return sq.Dump(), slices.SortBy(r, func(a ir.AgwResource) string {
			return a.ResourceName()
		})
	})
}

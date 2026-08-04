package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestStatusContributionsMergeRouteParentsAcrossGateways(t *testing.T) {
	route := types.NamespacedName{Namespace: "default", Name: "route"}
	gw1 := reports.ParentRefKey{NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw-1"}}
	gw2 := reports.ParentRefKey{NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw-2"}}

	first := reports.NewReportMap()
	first.HTTPRoutes[route] = &reports.RouteReport{Parents: map[reports.ParentRefKey]*reports.ParentRefReport{gw1: {}}}
	second := reports.NewReportMap()
	second.HTTPRoutes[route] = &reports.RouteReport{Parents: map[reports.ParentRefKey]*reports.ParentRefReport{gw2: {}}}

	contributions := append(
		reports.StatusContributionsFromReportMap(reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/gw-1"}, first),
		reports.StatusContributionsFromReportMap(reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/gw-2"}, second)...,
	)
	merged := reports.MergeStatusContributions(contributions)

	require.Equal(t, map[reports.ParentRefKey]*reports.ParentRefReport{gw1: {}, gw2: {}}, merged.HTTPRoutes[route].Parents)
}

func TestStatusContributionsMergePolicyAncestorsAcrossPaths(t *testing.T) {
	policy := reporter.PolicyKey{Group: "example.io", Kind: "Policy", Namespace: "default", Name: "policy"}
	gw := reports.ParentRefKey{NamespacedName: types.NamespacedName{Namespace: "default", Name: "gw"}}
	backend := reports.ParentRefKey{NamespacedName: types.NamespacedName{Namespace: "default", Name: "backend"}}

	gatewayReport := reports.NewReportMap()
	gatewayReport.Policies[policy] = &reports.PolicyReport{Ancestors: map[reports.ParentRefKey]*reports.AncestorRefReport{gw: {}}}
	backendReport := reports.NewReportMap()
	backendReport.Policies[policy] = &reports.PolicyReport{Ancestors: map[reports.ParentRefKey]*reports.AncestorRefReport{backend: {}}}

	contributions := append(
		reports.StatusContributionsFromReportMap(reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/gw"}, gatewayReport),
		reports.StatusContributionsFromReportMap(reports.StatusSource{Kind: reports.BackendPolicyStatusSource, Name: "default/backend"}, backendReport)...,
	)
	merged := reports.MergeStatusContributions(contributions)

	require.Equal(t, map[reports.ParentRefKey]*reports.AncestorRefReport{gw: {}, backend: {}}, merged.Policies[policy].Ancestors)
}

func TestGatewayTranslationOutputSeparatesStatusFromXdsEquality(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "gateway"}
	base := gatewayTranslationOutput{
		Xds: GatewayXdsResources{NamespacedName: nn},
		Status: GatewayStatusSnapshot{
			NamespacedName: nn,
			Contributions: []reports.StatusContribution{{
				Target: reports.StatusTarget{NamespacedName: nn},
				Source: reports.StatusSource{Kind: reports.GatewayStatusSource, Name: nn.String()},
			}},
		},
	}
	changed := base
	changed.Status.Contributions = []reports.StatusContribution{{
		Target: reports.StatusTarget{NamespacedName: nn},
		Source: reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "changed"},
	}}

	require.True(t, base.Xds.Equals(changed.Xds), "status-only changes must not invalidate the xDS projection")
	require.False(t, base.Equals(changed), "the status projection must still observe status-only changes")
}

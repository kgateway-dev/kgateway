package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestMergeProxyReports(t *testing.T) {
	tests := []struct {
		name     string
		proxies  []GatewayXdsResources
		expected reports.ReportMap
	}{
		{
			name: "Merge HTTPRoute reports for different parents",
			proxies: []GatewayXdsResources{
				{
					reports: reports.ReportMap{
						HTTPRoutes: map[types.NamespacedName]*reports.RouteReport{
							{Name: "route1", Namespace: "default"}: {
								Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
				{
					reports: reports.ReportMap{
						HTTPRoutes: map[types.NamespacedName]*reports.RouteReport{
							{Name: "route1", Namespace: "default"}: {
								Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
			},
			expected: reports.ReportMap{
				HTTPRoutes: map[types.NamespacedName]*reports.RouteReport{
					{Name: "route1", Namespace: "default"}: {
						Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
							{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
							{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
						},
					},
				},
			},
		},
		{
			name: "Merge TCPRoute reports for different parents",
			proxies: []GatewayXdsResources{
				{
					reports: reports.ReportMap{
						TCPRoutes: map[types.NamespacedName]*reports.RouteReport{
							{Name: "route1", Namespace: "default"}: {
								Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
				{
					reports: reports.ReportMap{
						TCPRoutes: map[types.NamespacedName]*reports.RouteReport{
							{Name: "route1", Namespace: "default"}: {
								Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
			},
			expected: reports.ReportMap{
				TCPRoutes: map[types.NamespacedName]*reports.RouteReport{
					{Name: "route1", Namespace: "default"}: {
						Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
							{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
							{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
						},
					},
				},
			},
		},
		{
			name: "Merge TLSRoute reports for different parents",
			proxies: []GatewayXdsResources{
				{
					reports: reports.ReportMap{
						TLSRoutes: map[types.NamespacedName]*reports.RouteReport{
							{Name: "route1", Namespace: "default"}: {
								Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
				{
					reports: reports.ReportMap{
						TLSRoutes: map[types.NamespacedName]*reports.RouteReport{
							{Name: "route1", Namespace: "default"}: {
								Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
			},
			expected: reports.ReportMap{
				TLSRoutes: map[types.NamespacedName]*reports.RouteReport{
					{Name: "route1", Namespace: "default"}: {
						Parents: map[reports.ParentRefKey]*reports.ParentRefReport{
							{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
							{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
						},
					},
				},
			},
		},
		{
			name: "Merge Policy reports for different parents",
			proxies: []GatewayXdsResources{
				{
					reports: reports.ReportMap{
						Policies: map[reporter.PolicyKey]*reports.PolicyReport{
							{Name: "policy1", Namespace: "default"}: {
								Ancestors: map[reports.ParentRefKey]*reports.AncestorRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
				{
					reports: reports.ReportMap{
						Policies: map[reporter.PolicyKey]*reports.PolicyReport{
							{Name: "policy1", Namespace: "default"}: {
								Ancestors: map[reports.ParentRefKey]*reports.AncestorRefReport{
									{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
								},
							},
						},
					},
				},
			},
			expected: reports.ReportMap{
				Policies: map[reporter.PolicyKey]*reports.PolicyReport{
					{Name: "policy1", Namespace: "default"}: {
						Ancestors: map[reports.ParentRefKey]*reports.AncestorRefReport{
							{NamespacedName: types.NamespacedName{Name: "gw-1", Namespace: "default"}}: {},
							{NamespacedName: types.NamespacedName{Name: "gw-2", Namespace: "default"}}: {},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			actual := mergeProxyReports(tt.proxies)
			if tt.expected.HTTPRoutes != nil {
				a.Equal(tt.expected.HTTPRoutes, actual.HTTPRoutes)
			}
			if tt.expected.TCPRoutes != nil {
				a.Equal(tt.expected.TCPRoutes, actual.TCPRoutes)
			}
			if tt.expected.TLSRoutes != nil {
				a.Equal(tt.expected.TLSRoutes, actual.TLSRoutes)
			}
			if tt.expected.Policies != nil {
				a.Equal(tt.expected.Policies, actual.Policies)
			}
		})
	}
}

package samples

import (
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// create a list of route tables where each one is a child of the next
func LinkedRouteTables(namespace, prefix, exact string) v1.RouteTableList {
	routeTables := v1.RouteTableList{
		{
			Metadata: core.Metadata{Name: "leaf", Namespace: namespace},
			Routes: []*v1.Route{
				{
					Matcher: &gloov1.Matcher{
						PathSpecifier: &gloov1.Matcher_Exact{
							Exact: exact,
						},
					},
					Action: &v1.Route_DirectResponseAction{DirectResponseAction: &gloov1.DirectResponseAction{}},
				},
			},
		},
	}
	// create a chain of route tables
	for i := 0; i < 5; i++ {
		ref := routeTables[i].Metadata.Ref()
		routeTables = append(routeTables, &v1.RouteTable{
			Metadata: core.Metadata{Name: fmt.Sprintf("node-%v", i), Namespace: namespace},
			Routes: []*v1.Route{{
				Matcher: &gloov1.Matcher{
					PathSpecifier: &gloov1.Matcher_Prefix{
						Prefix: prefix,
					},
				},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &ref,
				}},
			}})
	}

	return routeTables
}

func LinkedRouteTablesWithVirtualService(vsName, namespace, prefix, exact string) (*v1.VirtualService, v1.RouteTableList) {
	routeTables := v1.RouteTableList{
		{
			Metadata: core.Metadata{Name: "leaf", Namespace: namespace},
			Routes: []*v1.Route{
				{
					Matcher: &gloov1.Matcher{
						PathSpecifier: &gloov1.Matcher_Exact{
							Exact: exact,
						},
					},
					Action: &v1.Route_DirectResponseAction{DirectResponseAction: &gloov1.DirectResponseAction{}},
				},
			},
		},
	}
	// create a chain of route tables
	for i := 0; i < 5; i++ {
		ref := routeTables[i].Metadata.Ref()
		routeTables = append(routeTables, &v1.RouteTable{
			Metadata: core.Metadata{Name: fmt.Sprintf("node-%v", i), Namespace: namespace},
			Routes: []*v1.Route{{
				Matcher: &gloov1.Matcher{
					PathSpecifier: &gloov1.Matcher_Prefix{
						Prefix: prefix,
					},
				},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &ref,
				}},
			}})
	}
	ref := routeTables[len(routeTables)-1].Metadata.Ref()
	vs := defaults.DefaultVirtualService(namespace, vsName)
	vs.VirtualHost.Routes = []*v1.Route{{
		Matcher: &gloov1.Matcher{
			PathSpecifier: &gloov1.Matcher_Prefix{
				Prefix: "/",
			},
		},
		Action: &v1.Route_DelegateAction{
			DelegateAction: &ref,
		},
	}}

	return vs, routeTables
}

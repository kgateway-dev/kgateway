package validation

import (
	"context"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// TranslateK8sGatewayProxies builds an extremely basic "dummy" gloov1.Proxy object with the provided `res`
// attached in the correct location, i.e. if a RouteOption is passed it will be on the Route or if a VirtualHostOption is
// passed it will be on the VirtualHost
func TranslateK8sGatewayProxies(ctx context.Context, snap *gloosnapshot.ApiSnapshot, res resources.Resource) ([]*gloov1.Proxy, error) {
	us := gloov1.NewUpstream("default", "zzz_fake-upstream-for-gloo-validation")
	us.UpstreamType = &gloov1.Upstream_Static{
		Static: &static.UpstreamSpec{
			Hosts: []*static.Host{
				{Addr: "solo.io", Port: 80},
			},
		},
	}
	snap.UpsertToResourceList(us)

	routes := []*gloov1.Route{{
		Name: "route",
		Action: &gloov1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Single{
					Single: &gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: us.GetMetadata().Ref(),
						},
					},
				},
			},
		},
	}}

	aggregateListener := &gloov1.Listener{
		Name:        "aggregate-listener",
		BindAddress: "127.0.0.1",
		BindPort:    8082,
		ListenerType: &gloov1.Listener_AggregateListener{
			AggregateListener: &gloov1.AggregateListener{
				HttpResources: &gloov1.AggregateListener_HttpResources{
					VirtualHosts: map[string]*gloov1.VirtualHost{
						"vhost": {
							Name:    "vhost",
							Domains: []string{"*"},
							Routes:  routes,
						},
					},
				},
				HttpFilterChains: []*gloov1.AggregateListener_HttpFilterChain{{
					HttpOptionsRef:  "opts1",
					VirtualHostRefs: []string{"vhost"},
				}},
			},
		},
	}

	proxy := &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      "zzz-fake-proxy-for-validation",
			Namespace: "gloo-system",
		},
		Listeners: []*gloov1.Listener{
			aggregateListener,
		},
	}

	// add the policy object we are validating to the correct location
	switch policy := res.(type) {
	case *sologatewayv1.RouteOption:
		routes[0].Options = policy.GetOptions()
	case *sologatewayv1.VirtualHostOption:
		aggregateListener.GetAggregateListener().GetHttpResources().GetVirtualHosts()["vhost"].Options = policy.GetOptions()
	}

	return []*gloov1.Proxy{proxy}, nil
}

// GetSimpleErrorFromGlooValidation will get the Errors for the provided `proxy` that exist in the provided `reports`
// This function will return with the first error present if multiple reports exist
func GetSimpleErrorFromGlooValidation(
	reports []*gloovalidation.GlooValidationReport,
	proxy *gloov1.Proxy,
) error {
	var errs error

	for _, report := range reports {
		proxyResReport := report.ResourceReports[proxy]
		if proxyResReport.Errors != nil {
			return proxyResReport.Errors
		}
	}

	return errs
}

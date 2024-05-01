package testutils

import (
	"context"

	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/statusutils"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"github.com/solo-io/gloo/projects/gateway2/validation"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	k8scorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func BuildValidationHelper() validation.ValidationHelper {
	var (
		ctx               context.Context
		sch               *runtime.Scheme
		authConfigClient  extauth.AuthConfigClient
		routeOptionClient sologatewayv1.RouteOptionClient
		statusReporter    reporter.StatusReporter
		inputChannels     *proxy_syncer.GatewayInputChannels
	)

	ctx = context.Background()

	sch = scheme.NewScheme()
	resourceClientFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}

	routeOptionClient, _ = sologatewayv1.NewRouteOptionClient(ctx, resourceClientFactory)
	authConfigClient, _ = extauth.NewAuthConfigClient(ctx, resourceClientFactory)
	statusClient := statusutils.GetStatusClientForNamespace("gloo-system")

	statusReporter = reporter.NewReporter("gloo-kube-gateway", statusClient, routeOptionClient.BaseClient())

	inputChannels = proxy_syncer.NewGatewayInputChannels()

	deps := []client.Object{svc(), gw(), httpRoute(), attachedRouteOption()}
	routeOptionClient.Write(AttachedInternal(), clients.WriteOpts{})
	fakeClient := testutils.BuildIndexedFakeClient(deps, gwquery.IterateIndices, rtoptquery.IterateIndices)
	gwQueries := testutils.BuildGatewayQueriesWithClient(fakeClient)

	k8sGwExtensions, _ := extensions.NewK8sGatewayExtensions(ctx, extensions.K8sGatewayExtensionsFactoryParameters{
		Cl:                fakeClient,
		Scheme:            sch,
		AuthConfigClient:  authConfigClient,
		RouteOptionClient: routeOptionClient,
		StatusReporter:    statusReporter,
		KickXds:           inputChannels.Kick,
	})

	validator := validation.ValidationHelper{
		K8sGwExtensions: k8sGwExtensions,
		GatewayQueries:  gwQueries,
		Cl:              fakeClient,
	}
	return validator
}

func nsPtr(s string) *gwv1.Namespace {
	var ns gwv1.Namespace = gwv1.Namespace(s)
	return &ns
}

func portNumPtr(n int32) *gwv1.PortNumber {
	var pn gwv1.PortNumber = gwv1.PortNumber(n)
	return &pn
}

func svc() *k8scorev1.Service {
	return &k8scorev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "my-svc",
		},
		Spec: k8scorev1.ServiceSpec{
			Ports: []k8scorev1.ServicePort{
				{
					Port: 8080,
				},
			},
		},
	}
}

func httpRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-route",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name:      "my-gw",
						Namespace: nsPtr("default"),
					},
				},
			},
			Hostnames: []gwv1.Hostname{
				gwv1.Hostname("example.com"),
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: "my-svc",
									Port: portNumPtr(8080),
								},
							},
						},
					},
				},
			},
		},
	}
}

func gw() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name:     "my-http-listener",
					Port:     gwv1.PortNumber(8080),
					Protocol: gwv1.HTTPProtocolType,
					// AllowedRoutes: &gwv1.AllowedRoutes{
					// 	Namespaces: &gwv1.RouteNamespaces{
					// 		gw
					// 	},
					// },
				},
			},
		},
	}
}

func attachedRouteOption() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		TypeMeta: metav1.TypeMeta{
			Kind: sologatewayv1.RouteOptionGVK.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: *AttachedInternal(),
	}
}

func AttachedInternal() *sologatewayv1.RouteOption {
	return &sologatewayv1.RouteOption{
		Metadata: &core.Metadata{
			Name:      "policy",
			Namespace: "default",
		},
		TargetRef: &corev1.PolicyTargetReference{
			Group:     gwv1.GroupVersion.Group,
			Kind:      wellknown.HTTPRouteKind,
			Name:      "my-route",
			Namespace: wrapperspb.String("default"),
		},
		Options: &v1.RouteOptions{
			Faults: &faultinjection.RouteFaults{
				Abort: &faultinjection.RouteAbort{
					Percentage: 4.19,
					HttpStatus: 500,
				},
			},
		},
	}
}

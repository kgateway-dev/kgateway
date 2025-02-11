//go:build ignore

package headless_svc

import (
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1 "github.com/solo-io/solo-apis/pkg/api/gateway.solo.io/v1"
	soloapis_gloov1 "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1"
	"github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/core/matchers"
	soloapis_kubernetes "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/options/kubernetes"
	gloocore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"

	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/resources"
)

const (
	// Resources are defined as go structs and written to yaml files in the input directory
	K8sApiRoutingGeneratedFileName         = "k8s_api.gen.yaml"
	EdgeGatewayApiRoutingGeneratedFileName = "gloo_gateway_api.gen.yaml"

	headlessSvcDomain = "headless.example.com"
)

var (
	headlessSvcSetupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	// GetEdgeGatewayResources returns the Gloo Gateway Edge API resources
	GetEdgeGatewayResources = func(installNamespace string) []client.Object {
		headlessSvcUpstream := &soloapis_gloov1.Upstream{
			TypeMeta: metav1.TypeMeta{
				Kind:       gloov1.UpstreamGVK.Kind,
				APIVersion: fmt.Sprintf("%s/%s", gloov1.UpstreamGVK.Group, gloov1.UpstreamGVK.Version),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headless-nginx-upstream",
				Namespace: installNamespace,
			},
			Spec: soloapis_gloov1.UpstreamSpec{
				UpstreamType: &soloapis_gloov1.UpstreamSpec_Kube{
					Kube: &soloapis_kubernetes.UpstreamSpec{
						Selector: map[string]string{
							"app.kubernetes.io/name": "nginx",
						},
						ServiceName:      headlessService.GetName(),
						ServiceNamespace: headlessService.GetNamespace(),
						ServicePort:      8080,
					},
				},
			},
		}

		headlessVs := &v1.VirtualService{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1.VirtualServiceGVK.Kind,
				APIVersion: fmt.Sprintf("%s/%s", v1.VirtualServiceGVK.Group, v1.VirtualServiceGVK.Version),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headless-vs",
				Namespace: installNamespace,
			},
			Spec: v1.VirtualServiceSpec{
				VirtualHost: &v1.VirtualHost{
					Domains: []string{headlessSvcDomain},
					Routes: []*v1.Route{{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							},
						},
						Action: &v1.Route_RouteAction{
							RouteAction: &soloapis_gloov1.RouteAction{
								Destination: &soloapis_gloov1.RouteAction_Single{
									Single: &soloapis_gloov1.Destination{
										DestinationType: &soloapis_gloov1.Destination_Upstream{
											Upstream: &gloocore.ResourceRef{
												Name:      headlessSvcUpstream.Name,
												Namespace: headlessSvcUpstream.Namespace,
											},
										},
									},
								},
							},
						},
					}},
				},
			},
		}

		var resources []client.Object
		resources = append(resources, headlessVs, headlessSvcUpstream)
		return resources
	}

	K8sGateway = &gwv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       resources.K8sGatewayGvk.Kind,
			APIVersion: fmt.Sprintf("%s/%s", resources.K8sGatewayGvk.Group, resources.K8sGatewayGvk.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "gloo-gateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: "HTTP",
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: ptr.To(gwv1.NamespacesFromSame),
						},
					},
				},
			},
		},
	}

	// k8s Gateway API resources
	HeadlessSvcHTTPRoute = &gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       resources.HTTPRouteGvk.Kind,
			APIVersion: fmt.Sprintf("%s/%s", resources.HTTPRouteGvk.Group, resources.HTTPRouteGvk.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-httproute",
			Namespace: K8sGateway.GetNamespace(), // use the same namespace as the Gateway because NamespacesFromSame is set
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(K8sGateway.GetName()),
					},
				},
			},
			Hostnames: []gwv1.Hostname{gwv1.Hostname(headlessSvcDomain)},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: "headless-example-svc",
									Port: ptr.To(gwv1.PortNumber(8080)),
								},
							},
						},
					},
				},
			},
		},
	}
)

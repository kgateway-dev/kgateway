package istio

import (
	"fmt"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/solo-apis/pkg/api/gateway.solo.io/v1"
	soloapis_gloov1 "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1"
	"github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/core/matchers"
	soloapis_kubernetes "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/options/kubernetes"
	gloocore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	EdgeApisRoutingResourcesFileName = "edge-apis-routing.gen.yaml"

	httpbinSvc = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "httpbin", Namespace: "httpbin"}}

	// Edge API resources for no sslConfig on Upstream
	GetGlooGatewayEdgeResources = func(installNamespace string) []client.Object {
		httpbinUpstream := &soloapis_gloov1.Upstream{
			TypeMeta: metav1.TypeMeta{
				Kind:       gloov1.UpstreamGVK.Kind,
				APIVersion: fmt.Sprintf("%s/%s", gloov1.UpstreamGVK.Group, gloov1.UpstreamGVK.Version),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin-upstream",
				Namespace: installNamespace,
			},
			Spec: soloapis_gloov1.UpstreamSpec{
				UpstreamType: &soloapis_gloov1.UpstreamSpec_Kube{
					Kube: &soloapis_kubernetes.UpstreamSpec{
						Selector: map[string]string{
							"app": "httpbin",
						},
						ServiceName:      httpbinSvc.GetName(),
						ServiceNamespace: httpbinSvc.GetNamespace(),
						ServicePort:      8000,
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
				Name:      "httpbin-vs",
				Namespace: installNamespace,
			},
			Spec: v1.VirtualServiceSpec{
				VirtualHost: &v1.VirtualHost{
					Domains: []string{"httpbin"},
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
												Name:      httpbinUpstream.Name,
												Namespace: httpbinUpstream.Namespace,
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
		resources = append(resources, headlessVs, httpbinUpstream)
		return resources
	}
)

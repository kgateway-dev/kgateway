package gloo_test

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv1kubetypes "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gatewayv1kube "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/client/clientset/versioned/typed/gateway.solo.io/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1kubetypes "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	gloov1kube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/client/clientset/versioned/typed/gloo.solo.io/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Tests generated code in projects/gateway/pkg/api/v1/kube
var _ = Describe("Generated Kube Code", func() {
	var (
		glooV1Client    gloov1kube.GlooV1Interface       // upstreams
		gatewayV1Client gatewayv1kube.GatewayV1Interface // virtual service

		upstreamClient       gloov1.UpstreamClient
		virtualServiceClient gatewayv1.VirtualServiceClient
	)

	BeforeEach(func() {
		cfg, err := kubeutils.GetConfig("", "")
		Expect(err).NotTo(HaveOccurred())

		glooV1Client, err = gloov1kube.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		gatewayV1Client, err = gatewayv1kube.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		glooV1Client, err = gloov1kube.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		upstreamClient = resourceClientset.UpstreamClient()
		virtualServiceClient = resourceClientset.VirtualServiceClient()
	})

	It("can read and write a gloo resource as a typed kube object", func() {
		us := &gloov1kubetypes.Upstream{
			ObjectMeta: v1.ObjectMeta{Name: "petstore-static", Namespace: "default"},
			Spec: gloov1.Upstream{
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static.UpstreamSpec{
						Hosts: []*static.Host{{Addr: "petstore.swagger.io"}},
					},
				},
			},
		}

		vs := &gatewayv1kubetypes.VirtualService{
			ObjectMeta: v1.ObjectMeta{Name: "my-routes", Namespace: "default"},
			Spec: gatewayv1.VirtualService{
				VirtualHost: &gatewayv1.VirtualHost{
					Routes: []*gatewayv1.Route{{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"},
						}},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      us.Name,
												Namespace: us.Namespace,
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

		// this fixes a flake in v1.14.x. This flake occurs when we try to
		// `glooV1Client.Upstreams(us.Namespace).Create(ctx, us, v1.CreateOptions{})` create the resource.
		// I do not know why this resource already exists, but this fixes it.
		resourceName := "petstore-static"
		err := glooV1Client.Upstreams("default").Delete(ctx, resourceName, v1.DeleteOptions{})
		Expect(err).To(Or(Not(HaveOccurred()), MatchError(ContainSubstring("not found")), MatchError(ContainSubstring("already exists"))))
		resourceName = "my-routes"
		err = gatewayV1Client.VirtualServices("default").Delete(ctx, resourceName, v1.DeleteOptions{})
		Expect(err).To(Or(Not(HaveOccurred()), MatchError(ContainSubstring("not found")), MatchError(ContainSubstring("already exists"))))

		// ensure we can write the with kube clients

		_, err = glooV1Client.Upstreams(us.Namespace).Create(ctx, us, v1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = gatewayV1Client.VirtualServices(vs.Namespace).Create(ctx, vs, v1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		// ensure we can read with the solo-kit clients

		glooUpstream, err := upstreamClient.Read(us.Namespace, us.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(glooUpstream.UpstreamType).To(Equal(us.Spec.UpstreamType))

		glooVirtualService, err := virtualServiceClient.Read(vs.Namespace, vs.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(glooVirtualService.VirtualHost).To(Equal(vs.Spec.VirtualHost))
	})
})

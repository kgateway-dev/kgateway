package add_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Routes", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	BeforeEach(func() {
		err := testutils.Glooctl("create upstream static default-petstore-8080 --static-hosts jsonplaceholder.typicode.com:80")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	It("should create static upstream", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-1 --dest-name default-petstore-8080 --prefix-rewrite /api/pets")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
	})

	It("should create static upstream group", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-1 --upstream-group-name petstore --upstream-group-namespace default --prefix-rewrite /api/pets")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
		ug := vs.VirtualHost.Routes[0].GetRouteAction().GetUpstreamGroup()
		Expect(ug.GetName()).To(Equal("petstore"))
		Expect(ug.GetNamespace()).To(Equal("default"))
	})

	It("should take in headers", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-a --dest-name default-petstore-8080 --header param1=value1,param2=,param3=")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
		parameters := vs.VirtualHost.Routes[0].Matchers[0].Headers
		Expect(parameters[0].Name).To(Equal("param1"))
		Expect(parameters[0].Value).To(Equal("value1"))
		Expect(parameters[1].Name).To(Equal("param2"))
		Expect(parameters[1].Value).To(Equal(""))
		Expect(parameters[2].Name).To(Equal("param3"))
		Expect(parameters[2].Value).To(Equal(""))
	})

	It("should take in query parameters", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-a --dest-name default-petstore-8080 --queryParameter param1=value1,param2=,param3=")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "default", clients.ReadOpts{})
		Expect(vs.Metadata.Name).To(Equal("default"))
		parameters := vs.VirtualHost.Routes[0].Matchers[0].QueryParameters
		Expect(parameters[0].Name).To(Equal("param1"))
		Expect(parameters[0].Value).To(Equal("value1"))
		Expect(parameters[1].Name).To(Equal("param2"))
		Expect(parameters[1].Value).To(Equal(""))
		Expect(parameters[2].Name).To(Equal("param3"))
		Expect(parameters[2].Value).To(Equal(""))
	})

	It("should add route to route table", func() {
		err := testutils.Glooctl("add route --path-exact /sample-route-a --dest-name default-petstore-8080 --name=my-routes --to-route-table")
		Expect(err).NotTo(HaveOccurred())

		rt, err := helpers.MustRouteTableClient(ctx).Read("gloo-system", "my-routes", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rt.Routes[0]).To(Equal(&v1.Route{
			Matchers: []*matchers.Matcher{{
				PathSpecifier: &matchers.Matcher_Exact{
					Exact: "/sample-route-a",
				},
			}},
			Action: &v1.Route_RouteAction{
				RouteAction: &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_Single{
						Single: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      "default-petstore-8080",
									Namespace: "gloo-system",
								},
							},
						},
					},
				},
			},
		}))
	})

	It("should add delegate route", func() {
		err := testutils.Glooctl("add route --path-prefix /a --delegate-name my-delegate")
		Expect(err).NotTo(HaveOccurred())

		vs, err := helpers.MustVirtualServiceClient(ctx).Read("gloo-system", "default", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vs.GetVirtualHost().GetRoutes()).To(Equal([]*v1.Route{
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/a",
					},
				}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "my-delegate",
								Namespace: "gloo-system",
							},
						},
					},
				},
			},
		}))
	})
})

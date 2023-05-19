package surveyutils_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	. "github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Route", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())

		vsClient := helpers.MustVirtualServiceClient(ctx)
		vs := &gatewayv1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "vs",
				Namespace: "gloo-system",
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Routes: []*gatewayv1.Route{{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"},
					}}}, {
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/r"},
					}}},
				},
			},
		}
		_, err := vsClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		usClient := helpers.MustUpstreamClient(ctx)
		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "gloo-system.some-ns-test-svc-1234",
				Namespace: "gloo-system",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceName:      "test-svc",
					ServiceNamespace: "some-ns",
					ServicePort:      1234,
				},
			},
		}
		_, err = usClient.Write(us, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		mockLambdaFunctions := []*aws.LambdaFunctionSpec{{
			LogicalName:        "function1",
			LambdaFunctionName: "function1",
			Qualifier:          "",
		}}
		us2 := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "gloo-system.some-ns-test-svc-5678",
				Namespace: "gloo-system",
			},
			UpstreamType: &v1.Upstream_Aws{
				Aws: &aws.UpstreamSpec{
					Region: "some-region",
					SecretRef: &core.ResourceRef{
						Name:      "some-name",
						Namespace: "some-ns",
					},
					LambdaFunctions: mockLambdaFunctions,
				},
			},
		}

		_, err = usClient.Write(us2, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	It("should select a route", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("vsvc prompt:")
			c.SendLine("")
			c.ExpectString("route prompt:")
			c.PressDown()
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var opts options.Options
			_, idx, err := SelectRouteInteractive(&opts, "vsvc prompt:", "route prompt:")
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(1))
		})
	})

	It("should populate the correct flags", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("Choose a Virtual Service to add the route to")
			c.PressDown()
			c.SendLine("")
			c.ExpectString("where do you want to insert the route in the virtual service's route list?")
			c.SendLine("")
			c.ExpectString("Choose a path match type")
			c.SendLine("")
			c.ExpectString("What path prefix should we match?")
			c.SendLine("")
			c.ExpectString("Add a header matcher for this function (empty to skip)?")
			c.SendLine("")
			c.ExpectString("HTTP Method to match for this route (empty to skip)?")
			c.SendLine("")
			c.ExpectString("Choose the upstream or upstream group to route to:")
			c.SendLine("")
			c.ExpectString("do you wish to add a prefix-rewrite transformation to the route")
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var opts options.Options
			err := AddRouteFlagsInteractive(&opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.Metadata.Name).To(Equal("vs"))
			Expect(opts.Metadata.Namespace).To(Equal("gloo-system"))
			Expect(opts.Add.Route.Matcher.PathPrefix).To(Equal("/"))
			Expect(opts.Add.Route.Destination.Upstream.Namespace).To(Equal("gloo-system"))
			Expect(opts.Add.Route.Destination.Upstream.Name).To(Equal("gloo-system.some-ns-test-svc-1234"))

		})
	})

	It("should allow you to choose a function", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("Choose a Virtual Service to add the route to")
			c.PressDown()
			c.SendLine("")
			c.ExpectString("where do you want to insert the route in the virtual service's route list?")
			c.SendLine("")
			c.ExpectString("Choose a path match type")
			c.SendLine("")
			c.ExpectString("What path prefix should we match?")
			c.SendLine("")
			c.ExpectString("Add a header matcher for this function (empty to skip)?")
			c.SendLine("")
			c.ExpectString("HTTP Method to match for this route (empty to skip)?")
			c.SendLine("")
			c.ExpectString("Choose the upstream or upstream group to route to:")
			c.SendLine("gloo-system.some-ns-test-svc-5678")
			c.ExpectString("which function should this route invoke?")
			c.SendLine("function1")
			c.ExpectString("do you wish to add a prefix-rewrite transformation to the route")
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var opts options.Options
			err := AddRouteFlagsInteractive(&opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.Metadata.Name).To(Equal("vs"))
			Expect(opts.Metadata.Namespace).To(Equal("gloo-system"))
			Expect(opts.Add.Route.Matcher.PathPrefix).To(Equal("/"))
			Expect(opts.Add.Route.Destination.Upstream.Namespace).To(Equal("gloo-system"))
			Expect(opts.Add.Route.Destination.Upstream.Name).To(Equal("gloo-system.some-ns-test-svc-5678"))
			Expect(opts.Add.Route.Destination.DestinationSpec.Aws.LogicalName).To(Equal("function1"))

		})
	})

	It("should allow you to skip choosing a function", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("Choose a Virtual Service to add the route to")
			c.PressDown()
			c.SendLine("")
			c.ExpectString("where do you want to insert the route in the virtual service's route list?")
			c.SendLine("")
			c.ExpectString("Choose a path match type")
			c.SendLine("")
			c.ExpectString("What path prefix should we match?")
			c.SendLine("")
			c.ExpectString("Add a header matcher for this function (empty to skip)?")
			c.SendLine("")
			c.ExpectString("HTTP Method to match for this route (empty to skip)?")
			c.SendLine("")
			c.ExpectString("Choose the upstream or upstream group to route to:")
			c.SendLine("gloo-system.some-ns-test-svc-5678")
			c.ExpectString("which function should this route invoke?")
			c.SendLine(NoneOfTheAbove)
			c.ExpectString("do you wish to add a prefix-rewrite transformation to the route")
			c.SendLine("/api/pets")
			c.ExpectEOF()
		}, func() {
			var opts options.Options
			err := AddRouteFlagsInteractive(&opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.Metadata.Name).To(Equal("vs"))
			Expect(opts.Metadata.Namespace).To(Equal("gloo-system"))
			Expect(opts.Add.Route.Matcher.PathPrefix).To(Equal("/"))
			Expect(opts.Add.Route.Destination.Upstream.Namespace).To(Equal("gloo-system"))
			Expect(opts.Add.Route.Destination.Upstream.Name).To(Equal("gloo-system.some-ns-test-svc-5678"))
			Expect(opts.Add.Route.Destination.DestinationSpec.Aws.LogicalName).To(Equal(NoneOfTheAbove))
			Expect(opts.Add.Route.Plugins.PrefixRewrite.Value).NotTo(Equal(nil))

		})
	})
})

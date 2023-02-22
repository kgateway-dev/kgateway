package route_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	routehelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Sort", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	It("should sort the routes on a virtual service", func() {
		sortedRoutes := func() []*gatewayv1.Route {
			var routes []*gatewayv1.Route
			for _, path := range []int{routehelpers.ExactPath, routehelpers.RegexPath, routehelpers.PrefixPath} {
				for _, length := range []int{9, 6, 3} {
					routes = append(routes, routehelpers.MakeGatewayRoute(path, length))
				}
			}
			return routes
		}

		unsortedRoutes := func() []*gatewayv1.Route {
			var routes []*gatewayv1.Route
			for _, length := range []int{9, 6, 3} {
				for _, path := range []int{routehelpers.ExactPath, routehelpers.RegexPath, routehelpers.PrefixPath} {
					routes = append(routes, routehelpers.MakeGatewayRoute(path, length))
				}
			}
			return routes
		}

		vs, err := helpers.MustVirtualServiceClient(ctx).Write(&gatewayv1.VirtualService{
			Metadata: &core.Metadata{Namespace: "gloo-system", Name: "tacos"},
			VirtualHost: &gatewayv1.VirtualHost{
				Routes: unsortedRoutes(),
			},
		}, clients.WriteOpts{Ctx: context.TODO()})
		Expect(err).NotTo(HaveOccurred())

		err = testutils.Glooctl(fmt.Sprintf("route sort --name %v --namespace %v", vs.Metadata.Name, vs.Metadata.Namespace))
		Expect(err).NotTo(HaveOccurred())

		vs, err = helpers.MustVirtualServiceClient(ctx).Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vs.VirtualHost.Routes).To(Equal(sortedRoutes()))
	})
})

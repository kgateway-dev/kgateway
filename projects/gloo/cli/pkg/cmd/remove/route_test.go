package remove_test

import (
	"context"
	"fmt"

	routehelpers "github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Remove Route", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should remove a route from a virtualservice", func() {
		vs, err := helpers.MustVirtualServiceClient().Write(&gatewayv1.VirtualService{
			Metadata: core.Metadata{Namespace: "gloo-system", Name: "tacos"},
			VirtualHost: &v1.VirtualHost{
				Routes: []*v1.Route{routehelpers.MakeRoute(routehelpers.RegexPath, 5)},
			},
		}, clients.WriteOpts{Ctx: context.TODO()})
		Expect(err).NotTo(HaveOccurred())

		err = testutils.Glooctl(fmt.Sprintf("remove route --name %v --namespace %v -x 0", vs.Metadata.Name, vs.Metadata.Namespace))
		Expect(err).NotTo(HaveOccurred())

		vs, err = helpers.MustVirtualServiceClient().Read(vs.Metadata.Namespace, vs.Metadata.Name, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vs.VirtualHost.Routes).To(HaveLen(0))
	})
})

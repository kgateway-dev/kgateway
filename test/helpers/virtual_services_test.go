package helpers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/test/matchers"

	"reflect"
)

var _ = Describe("VirtualServiceBuilder", func() {

	It("will fail if the virtual service builder has a new top level field", func() {
		// This test is important as it checks whether the virtual service builder has a new top level field.
		// This should happen very rarely, and should be used as an indication that the `Clone` function
		// most likely needs to change to support this new field

		Expect(reflect.TypeOf(helpers.VirtualServiceBuilder{}).NumField()).To(
			Equal(6),
			"wrong number of fields found",
		)
	})

	It("clones all fields", func() {
		originalBuilder := helpers.NewVirtualServiceBuilder().
			WithName("original-name").
			WithNamespace("original-namespace").
			WithDomain("original.com").
			WithVirtualHostOptions(&gloov1.VirtualHostOptions{
				Cors: &cors.CorsPolicy{
					AllowCredentials: true,
				},
			}).
			WithRoute("original-route", &gatewayv1.Route{
				Name: "original-route",
			}).
			WithSslConfig(&ssl.SslConfig{
				SniDomains: []string{"original.com"},
			})

		clonedBuilder := originalBuilder.Clone().
			WithName("cloned-name").
			WithNamespace("cloned-namespace").
			WithDomain("cloned.com").
			WithVirtualHostOptions(&gloov1.VirtualHostOptions{
				Cors: &cors.CorsPolicy{
					AllowCredentials: false,
				},
			}).
			// It's important that we update the original-route with a new name
			WithRoute("original-route", &gatewayv1.Route{
				Name: "cloned-route",
			}).
			WithSslConfig(&ssl.SslConfig{
				SniDomains: []string{"cloned.com"},
			})

		// Cloning the originalBuilder means we should be modifying only the clone, and not the original originalBuilder
		// Below we assert that each field in the generated VirtualService matches the value from the builder

		originalVirtualService := originalBuilder.Build()
		clonedVirtualService := clonedBuilder.Build()
		Expect(originalVirtualService.GetMetadata().GetName()).To(Equal("original-name"))
		Expect(clonedVirtualService.GetMetadata().GetName()).To(Equal("cloned-name"))

		Expect(originalVirtualService.GetMetadata().GetNamespace()).To(Equal("original-namespace"))
		Expect(clonedVirtualService.GetMetadata().GetNamespace()).To(Equal("cloned-namespace"))

		Expect(originalVirtualService.GetSslConfig().GetSniDomains()).To(Equal([]string{"original.com"}))
		Expect(clonedVirtualService.GetSslConfig().GetSniDomains()).To(Equal([]string{"cloned.com"}))

		originalVirtualHost := originalVirtualService.GetVirtualHost()
		clonedVirtualHost := clonedVirtualService.GetVirtualHost()
		Expect(originalVirtualHost.GetDomains()).To(Equal([]string{"original.com"}))
		Expect(clonedVirtualHost.GetDomains()).To(Equal([]string{"cloned.com"}))

		Expect(originalVirtualHost.GetOptions().GetCors().GetAllowCredentials()).To(BeTrue())
		Expect(clonedVirtualHost.GetOptions().GetCors().GetAllowCredentials()).To(BeFalse())

		Expect(originalVirtualHost.GetRoutes()).To(HaveLen(1))
		Expect(originalVirtualHost.GetRoutes()[0]).To(matchers.MatchProto(&gatewayv1.Route{
			Name: "original-route",
		}))
		Expect(originalVirtualHost.GetRoutes()).To(HaveLen(1))
		Expect(clonedVirtualHost.GetRoutes()[0]).To(matchers.MatchProto(&gatewayv1.Route{
			Name: "cloned-route",
		}))
	})

})

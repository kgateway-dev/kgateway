package kube_e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/pborman/uuid"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	extensions "github.com/solo-io/gloo/pkg/coreplugins/route-extensions"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Route Exetnsion - CORS", func() {
	const helloService = "helloservice"
	const servicePort = 8080

	Context("route with CORS", func() {
		servicePath := "/" + uuid.New()
		vhostName := "cors-route"
		BeforeEach(func() {
			_, err := gloo.V1().Upstreams().Create(&v1.Upstream{
				Name: helloService,
				Type: service.UpstreamTypeService,
				Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
					Hosts: []service.Host{
						{
							Addr: helloService,
							Port: servicePort,
						},
					},
				}),
			})
			Must(err)
			_, err = gloo.V1().VirtualHosts().Create(&v1.VirtualHost{
				Name: vhostName,
				Routes: []*v1.Route{{
					Matcher: &v1.Route_RequestMatcher{
						RequestMatcher: &v1.RequestMatcher{
							Path: &v1.RequestMatcher_PathExact{
								PathExact: servicePath,
							},
							Verbs: []string{"HEAD", "GET"},
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: helloService,
							},
						},
					},
					Extensions: extensions.EncodeRouteExtensionSpec(
						extensions.RouteExtensionSpec{
							Cors: &extensions.CorsPolicy{
								AllowOrigin:  []string{"*"},
								AllowMethods: "GET, POST",
							},
						},
					),
				}},
			})
			Must(err)
		})

		AfterEach(func() {
			gloo.V1().Upstreams().Delete(helloService)
			gloo.V1().VirtualHosts().Delete(vhostName)
		})

		It("should return response with CORS allow method header", func() {
			curlEventuallyShouldRespond(curlOpts{path: servicePath, returnHeaders: true},
				"access-control-allow-methods: GET, POST", time.Minute*5)
		})
		It("should return response with CORS allow origin header", func() {
			curlEventuallyShouldRespond(curlOpts{path: servicePath, returnHeaders: true},
				"access-control-allow-origin: *", time.Minute*5)
		})
	})
})

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
								AllowMethods: "GET, POST, PUT",
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

		FIt("should return response with CORS allow method header", func() {
			curlEventuallyShouldRespond(curlOpts{
				path:          servicePath,
				method:        "OPTIONS",
				returnHeaders: true,
				headers: map[string]string{
					"Origin":                        "gloo.solo.io",
					"Access-Control-Request-Method": "POST",
				}},
				"access-control-allow-methods: GET, POST, PUT", time.Minute*1)
		})
		FIt("should return response with CORS allow origin header", func() {
			curlEventuallyShouldRespond(curlOpts{
				path:          servicePath,
				method:        "OPTIONS",
				returnHeaders: true,
				headers: map[string]string{
					"Origin":                        "gloo.solo.io",
					"Access-Control-Request-Method": "POST",
				}},
				"access-control-allow-origin: gloo.solo.io", time.Minute*5)
		})
	})
})

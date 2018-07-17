package kube_e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/pborman/uuid"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/routing"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Route Exetnsion - CORS", func() {
	const helloService = "helloservice"
	const servicePort = 8080

	Context("route with CORS", func() {
		servicePath := "/" + uuid.New()
		vServiceName := "cors-route"
		BeforeEach(func() {
			_, err := gloo.V1().Upstreams().Create(&v1.Upstream{
				Name: helloService,
				Type: static.UpstreamTypeStatic,
				Spec: static.EncodeUpstreamSpec(&static.UpstreamSpec{
					Hosts: []*static.Host{
						{
							Addr: helloService,
							Port: servicePort,
						},
					},
				}),
			})
			Must(err)
			_, err = gloo.V1().VirtualServices().Create(&v1.VirtualService{
				Name: vServiceName,
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
					Extensions: routing.EncodeRouteExtensionSpec(
						&routing.RouteExtensions{
							Cors: &routing.CorsPolicy{
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
			gloo.V1().VirtualServices().Delete(vServiceName)
		})

		It("should return response with CORS allow method header", func() {
			curlEventuallyShouldRespond(curlOpts{
				path:          servicePath,
				method:        "OPTIONS",
				returnHeaders: true,
				headers: map[string]string{
					"Origin":                        "gloo.solo.io",
					"Access-Control-Request-Method": "POST",
				}},
				"access-control-allow-methods: GET, POST, PUT", time.Minute*5)
		})
		It("should return response with CORS allow origin header", func() {
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

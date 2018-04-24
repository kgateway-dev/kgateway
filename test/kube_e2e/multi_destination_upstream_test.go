package kube_e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/pborman/uuid"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Multiple Upstream Destinations", func() {
	const helloService = "helloservice"
	const helloService2 = "helloservice-2"
	const servicePort = 8080
	Context("creating a vService route with mutliple upstream destinations", func() {
		randomPath := "/" + uuid.New()
		vServiceName := "multidestinationroute"
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
			_, err = gloo.V1().Upstreams().Create(&v1.Upstream{
				Name: helloService2,
				Type: service.UpstreamTypeService,
				Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
					Hosts: []service.Host{
						{
							Addr: helloService2,
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
								PathExact: randomPath,
							},
							Verbs: []string{"GET"},
						},
					},
					MultipleDestinations: []*v1.WeightedDestination{
						{
							Destination: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &v1.UpstreamDestination{
										Name: helloService,
									},
								},
							},
							Weight: 1,
						},
						{
							Destination: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: &v1.UpstreamDestination{
										Name: helloService2,
									},
								},
							},
							Weight: 1,
						},
					},
				}},
			})
			Must(err)
		})
		AfterEach(func() {
			gloo.V1().Upstreams().Delete(helloService)
			gloo.V1().Upstreams().Delete(helloService2)
			gloo.V1().VirtualServices().Delete(vServiceName)
		})
		It("should balance requests between the two destinations", func() {
			curlEventuallyShouldRespond(curlOpts{path: randomPath}, "expected-reply-1", time.Minute*5)
			curlEventuallyShouldRespond(curlOpts{path: randomPath}, "expected-reply-2", time.Minute*5)
		})
	})
})

package nomad_e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/pborman/uuid"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Multiple Upstream Destinations", func() {
	const helloService = "helloservice"
	const helloService2 = "helloservice-2"
	Context("creating a vhost route with mutliple upstream destinations", func() {
		randomPath := "/" + uuid.New()
		vhostName := "multidestinationroute"
		BeforeEach(func() {
			_, err = gloo.V1().VirtualHosts().Create(&v1.VirtualHost{
				Name: vhostName,
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
			gloo.V1().VirtualHosts().Delete(vhostName)
		})
		It("should balance requests between the two destinations", func() {
			CurlEventuallyShouldRespond(CurlOpts{Path: randomPath}, "expected-reply-1", time.Second*35)
			CurlEventuallyShouldRespond(CurlOpts{Path: randomPath}, "expected-reply-2", time.Second*35)
		})
	})
})

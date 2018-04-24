package kube_e2e

import (
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/pborman/uuid"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Kubernetes Upstream + Endpoint Discovery Plugin", func() {
	const helloService = "helloservice"
	const servicePort = 8080
	Context("creating kube upstream and a vService with a single route to it", func() {
		randomPath := "/" + uuid.New()
		vServiceName := "one-route"
		BeforeEach(func() {
			_, err := gloo.V1().Upstreams().Create(&v1.Upstream{
				Name: helloService,
				Type: kubernetes.UpstreamTypeKube,
				Spec: kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
					ServiceNamespace: namespace,
					ServiceName:      helloService,
					ServicePort:      servicePort,
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
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: helloService,
							},
						},
					},
				}},
			})
			Must(err)
		})
		AfterEach(func() {
			gloo.V1().Upstreams().Delete(helloService)
			gloo.V1().VirtualServices().Delete(vServiceName)
		})
		It("should configure envoy with a 200 OK route (backed by helloservice)", func() {
			curlEventuallyShouldRespond(curlOpts{path: randomPath}, "< HTTP/1.1 200", time.Minute*5)
		})
	})
})

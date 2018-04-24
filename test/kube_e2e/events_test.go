package kube_e2e

import (
	"time"

	. "github.com/onsi/gomega"

	"strings"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	"github.com/solo-io/gloo/pkg/log"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Event matcher route type", func() {
	const upstreamForEvents = "upstream-for-events"
	const servicePort = 8080
	Context("routes events for the given topic", func() {
		vServiceName := "eventroute"
		BeforeEach(func() {
			_, err := gloo.V1().Upstreams().Create(&v1.Upstream{
				Name: upstreamForEvents,
				Type: service.UpstreamTypeService,
				Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
					Hosts: []service.Host{
						{
							Addr: upstreamForEvents,
							Port: servicePort,
						},
					},
				}),
			})
			Must(err)
			_, err = gloo.V1().VirtualServices().Create(&v1.VirtualService{
				Name: vServiceName,
				Routes: []*v1.Route{{
					Matcher: &v1.Route_EventMatcher{
						EventMatcher: &v1.EventMatcher{
							EventType: "test-topic",
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: upstreamForEvents,
							},
						},
					},
				}},
			})
			Must(err)
		})
		AfterEach(func() {
			gloo.V1().Upstreams().Delete(upstreamForEvents)
			gloo.V1().VirtualServices().Delete(vServiceName)
		})
		It("receive the event", func() {
			// start the event emitter
			resp, err := curl(curlOpts{path: "/start", service: "event-emitter"})
			Expect(err).To(BeNil())
			Expect(resp).To(ContainSubstring("HTTP/1.1 200"))
			logsShouldEventuallyContain(`"message":"what an event!"`, time.Minute*5)
		})
	})
})

func logsShouldEventuallyContain(substr string, timeout ...time.Duration) {
	t := time.Second * 20
	if len(timeout) > 0 {
		t = timeout[0]
	}
	// for some useful-ish output
	tick := time.Tick(t / 8)
	Eventually(func() string {
		logs, err := KubectlOut("logs", "-l", "gloo=upstream-for-events")
		if err != nil {
			logs = err.Error()
		}
		select {
		default:
			break
		case <-tick:
			log.GreyPrintf("logs for upstream: %v", logs)
		}
		if strings.Contains(logs, substr) {
			log.GreyPrintf("success: %v", logs)
		}
		return logs
	}, t, "1s").Should(ContainSubstring(substr))
}

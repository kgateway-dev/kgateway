package kube_e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/rest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

var _ = Describe("Project Fn Function Discovery Service Detection", func() {

	const vServiceName = "projectfn"
	const funcPath = "/api/func"

	var expectedServiceInfo *v1.ServiceInfo = &v1.ServiceInfo{Type: rest.ServiceTypeREST}
	var upstreamName string

	BeforeEach(func() {
		var options meta_v1.GetOptions
		svc, err := kube.CoreV1().Services("default").Get("my-release-fn-api", options)
		if err != nil || svc == nil {
			Skip("this test must be run with fission pre-installed")
		}
		upstreamName = "default-my-release-fn-api-80"
	})

	It("should detect the upstream service info", func() {
		Eventually(func() (*v1.ServiceInfo, error) {
			list, err := gloo.V1().Upstreams().List()
			if err != nil {
				return nil, err
			}
			var upstreamToTest *v1.Upstream
			for _, us := range list {
				if us.Name == upstreamName {
					upstreamToTest = us
					break
				}
			}

			if upstreamToTest == nil {
				return nil, errors.New("could not find upstream " + upstreamName)
			}
			return upstreamToTest.ServiceInfo, nil
		}, "2m", "5s").Should(Equal(expectedServiceInfo))
	})
	Context("test functions", func() {

		BeforeEach(func() {
			_, err := gloo.V1().VirtualServices().Create(&v1.VirtualService{
				Name: vServiceName,
				Routes: []*v1.Route{
					{
						Matcher: &v1.Route_RequestMatcher{
							RequestMatcher: &v1.RequestMatcher{
								Path: &v1.RequestMatcher_PathPrefix{
									PathPrefix: funcPath,
								},
								Verbs: []string{"GET"},
							},
						},
						SingleDestination: &v1.Destination{
							DestinationType: &v1.Destination_Function{
								Function: &v1.FunctionDestination{
									FunctionName: "myapp:hello-go",
									UpstreamName: upstreamName,
								},
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			gloo.V1().VirtualServices().Delete(vServiceName)
		})

		It("should receive hello world message", func() {
			curlEventuallyShouldRespond(curlOpts{
				path: funcPath,
			}, "Hello from Fn", time.Minute*5)
		})

	})
})

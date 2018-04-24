package local_e2e

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/pkg/api/types/v1"

	"time"

	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/local_e2e/test_grpc_service/glootest/protos"
)

var _ = Describe("GRPC Plugin", func() {
	It("Routes to GRPC Functions", func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()

		tu := NewTestGRPCUpstream(envoyInstance.LocalAddr(), glooInstance.FilesDir())
		err = glooInstance.AddUpstream(tu.Upstream)
		Expect(err).NotTo(HaveOccurred())

		v := &v1.VirtualService{
			Name: "default",
			Routes: []*v1.Route{
				{
					Matcher: &v1.Route_RequestMatcher{
						RequestMatcher: &v1.RequestMatcher{
							Path: &v1.RequestMatcher_PathExact{PathExact: "/test"},
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Function{
							Function: &v1.FunctionDestination{
								UpstreamName: tu.Upstream.Name,
								FunctionName: "TestMethod",
							},
						},
					},
				},
			},
		}

		err = glooInstance.AddVhost(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`{"str": "foo"}`)

		time.Sleep(time.Second)

		testRequest := func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			res, err := http.Post(fmt.Sprintf("http://%s:%d/test", "localhost", envoyPort), "application/json", &buf)
			if err == nil {
				b, _ := ioutil.ReadAll(res.Body)
				log.Printf("%v", res.Header)
				log.Printf("%v", res.Status)
				log.Printf("%v", string(b))
			}
			return err
		}

		// wait for envoy to start receiving request
		Eventually(testRequest, 60, 1).Should(BeNil())

		ch := make(chan struct{})
		go func() {
			for {
				select {
				case <-time.After(time.Second):
					err := testRequest()
					helpers.Must(err)
				case <-ch:
					return
				}
			}
		}()

		expectedResponse := &ReceivedRequest{
			GRPCRequest: &glootest.TestRequest{Str: "foo"},
		}
		Eventually(tu.C, time.Second*15).Should(Receive(Equal(expectedResponse)))
		close(ch)
	})

})

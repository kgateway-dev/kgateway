package transforms_test

import (
	"net/http"

	"github.com/solo-io/gloo/test/gomega/transforms"

	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Curl", func() {

	var (
		validResponse = "*   Trying 10.96.92.168...\n* TCP_NODELAY set\n* Connected to gateway-proxy (10.96.92.168) port 80 (#0)\n> GET /test HTTP/1.1\n> Host: test-domain\n> User-Agent: curl/7.58.0\n> Accept: */*\n> header1: value1\n> header2: value2\n> instructions: invalid json value\n> \n< HTTP/1.1 200 OK\n< x-powered-by: Express\n< content-type: application/json; charset=utf-8\n< content-length: 444\n< etag: W/\"1bc-u/C5Wu/6BvNtW0jEh2E+mCP4gUg\"\n< date: Thu, 28 Mar 2024 19:40:18 GMT\n< x-envoy-upstream-service-time: 374\n< server: envoy\n< \n{ [444 bytes data]\n* Connection #0 to host gateway-proxy left intact"
	)

	Describe("WithCurlHttpResponse", func() {

		It("matches valid response", func() {
			Expect(validResponse).To(
				WithTransform(transforms.WithCurlHttpResponse,
					matchers.HaveHttpResponse(&matchers.HttpResponse{
						StatusCode: http.StatusOK,
						Body:       gstruct.Ignore(),
					})))
		})
	})

})

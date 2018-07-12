package routing_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"fmt"
	"time"

	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/types"
	. "github.com/solo-io/gloo/pkg/coreplugins/routing"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/protoutil"
)

var _ = Describe("Spec", func() {
	It("decodes and uncodes from yaml", func() {
		timeout := time.Minute
		spec := &RouteExtensions{
			MaxRetries: 1,
			Timeout:    &timeout,
			AddRequestHeaders: []*HeaderValue{
				{
					Key:   "FOO",
					Value: "BAR",
				},
			},
		}
		encoded := EncodeRouteExtensionSpec(spec)
		jsn, err := protoutil.Marshal(encoded)
		Expect(err).NotTo(HaveOccurred())
		yam, err := yaml.JSONToYAML(jsn)
		Expect(err).NotTo(HaveOccurred())
		fmt.Printf("%s\n", yam)
	})
	It("decodes from yaml", func() {
		yam := `
add_request_headers:
- key: FOO
  value: BAR
max_retries: 1
timeout: 60.000s
something_invalid: another_spec_maybe?
cors:
  allow_origin: 
  - solo.io
  allow_methods: GET, POST
  allow_headers: X-Gloo, Ping
  max_age: 86400000000000
  allow_credentials: true`
		jsn, err := yaml.YAMLToJSON([]byte(yam))
		Expect(err).NotTo(HaveOccurred())
		var struc types.Struct
		err = protoutil.Unmarshal(jsn, &struc)
		Expect(err).NotTo(HaveOccurred())
		specc, err := DecodeRouteExtensions(&struc)
		Expect(err).NotTo(HaveOccurred())
		Expect(specc.Cors).NotTo(BeNil())
		Expect(specc.Cors.MaxAge).To(Equal(time.Duration(24 * time.Hour)))
		log.Printf("%v", specc)
	})
})

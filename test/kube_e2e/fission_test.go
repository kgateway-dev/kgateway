package kube_e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/rest"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Fission Function Discovery Service Detection", func() {

	var expectedServiceInfo *v1.ServiceInfo = &v1.ServiceInfo{Type: rest.ServiceTypeREST}
	var upstreamName string

	BeforeEach(func() {
		var options meta_v1.GetOptions
		svc, err := kube.CoreV1().Services("fission").Get("router", options)
		if err != nil || svc == nil {
			Skip("this test must be run with fission pre-installed")
		}
		upstreamName = "fission-router-80"
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

})

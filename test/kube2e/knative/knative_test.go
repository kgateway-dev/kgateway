package knative_test

import (
	"path/filepath"
	"time"

	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo/v2"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kube2e: Knative-Ingress", func() {

	BeforeEach(func() {
		deployKnativeTestService(knativeTestServiceFile())
	})

	AfterEach(func() {
		if err := deleteKnativeTestService(knativeTestServiceFile()); err != nil {
			log.Warnf("teardown failed %v", err)
		}
	})

	It("works", func() {
		ingressProxy := "knative-external-proxy"
		ingressPort := 80
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "http",
			Path:              "/",
			Method:            "GET",
			Host:              "helloworld-go.default.example.com",
			Service:           ingressProxy,
			Port:              ingressPort,
			ConnectionTimeout: 1,
			Verbose:           true,
		}, "Hello Go Sample v1!", 1, time.Minute*2, 1*time.Second)
	})
})

func knativeTestServiceFile() string {
	return filepath.Join(testHelper.RootDir, "test", "kube2e", "knative", "artifacts", "knative-hello-service.yaml")
}

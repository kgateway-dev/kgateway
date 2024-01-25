package helper_test

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/solo-io/gloo/test/kube2e/helper"

	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	kube2 "github.com/solo-io/k8s-utils/testutils/kube"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("test container tests", func() {

	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}

	var (
		ctx       context.Context
		namespace string
		kube      kubernetes.Interface
	)

	BeforeSuite(func() {
		ctx = context.Background()
		namespace = testutils.RandString(8)
		kube = kube2.MustKubeClient()
		err := kubeutils.CreateNamespacesInParallel(ctx, kube, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterSuite(func() {
		err := kubeutils.DeleteNamespacesInParallelBlocking(ctx, kube, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("test server", func() {
		var (
			testServer TestUpstreamServer
		)
		BeforeEach(func() {
			var err error
			testServer, err = NewTestServer(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Currently this DeployResources call takes 4 seconds in CI. If this
			// timeout is exceeded, we should look into why the http echo pod is
			// taking so long to spin up.
			err = testServer.DeployResources(time.Second * 30)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err := testServer.TerminatePodAndDeleteService()
			Expect(err).NotTo(HaveOccurred())
		})
		It("can install and uninstall the testserver", func() {
			host := fmt.Sprintf("%s.%s.svc.cluster.local:%v", TestServerName, namespace, TestServerPort)
			testServer.CurlEventuallyShouldRespond(CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              host,
				Service:           TestServerName,
				Port:              TestServerPort,
				ConnectionTimeout: 10,
			}, SimpleHttpResponse, 1, 120*time.Second)
		})
	})

	Context("http echo", func() {
		var (
			httpEcho TestContainer
		)
		BeforeEach(func() {
			var err error
			httpEcho, err = NewEchoHttp(namespace)
			Expect(err).NotTo(HaveOccurred())
			err = httpEcho.DeployResources(time.Minute)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err := httpEcho.TerminatePodAndDeleteService()
			Expect(err).NotTo(HaveOccurred())
		})
		It("can install and uninstall the http echo pod", func() {
			responseString := fmt.Sprintf(`"host":"%s.%s.svc.cluster.local:%v"`,
				HttpEchoName, namespace, HttpEchoPort)
			host := fmt.Sprintf("%s.%s.svc.cluster.local:%v", HttpEchoName, namespace, HttpEchoPort)
			httpEcho.CurlEventuallyShouldRespond(CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              host,
				Service:           HttpEchoName,
				Port:              HttpEchoPort,
				ConnectionTimeout: 10,
				Verbose:           true,
			}, responseString, 1, 120*time.Second)
		})
	})

	Context("tcp ehco", func() {
		var (
			tcpEcho TestContainer
		)
		BeforeEach(func() {
			var err error
			tcpEcho, err = NewEchoTcp(namespace)
			Expect(err).NotTo(HaveOccurred())
			err = tcpEcho.DeployResources(time.Minute)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err := tcpEcho.TerminatePodAndDeleteService()
			Expect(err).NotTo(HaveOccurred())
		})
		It("can install and uninstall the tcp echo pod", func() {
			responseString := fmt.Sprintf("Connected to %s",
				TcpEchoName)
			tcpEcho.CurlEventuallyShouldOutput(CurlOpts{
				Protocol:          "telnet",
				Service:           TcpEchoName,
				Port:              TcpEchoPort,
				ConnectionTimeout: 10,
				Verbose:           true,
			}, responseString, 1, 30*time.Second)
		})
	})
})

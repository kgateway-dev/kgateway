package helper

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	kube2 "github.com/solo-io/k8s-utils/testutils/kube"
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
	Context("test runner", func() {
		var (
			testRunner *testRunner
		)
		BeforeEach(func() {
			var err error
			testRunner, err = NewTestRunner(namespace)
			Expect(err).NotTo(HaveOccurred())
			err = testRunner.Deploy(time.Minute * 2)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err := testRunner.Terminate()
			Expect(err).NotTo(HaveOccurred())
		})
		It("can install and uninstall the testrunner", func() {
			// responseString := fmt.Sprintf(`"%s":"%s.%s.svc.cluster.local:%v"`,
			// 	linkerd.HeaderKey, helper.HttpEchoName, testHelper.InstallNamespace, helper.HttpEchoPort)
			host := fmt.Sprintf("%s.%s.svc.cluster.local:%v", TestrunnerName, namespace, TestRunnerPort)
			testRunner.CurlEventuallyShouldRespond(CurlOpts{
				Protocol:          "http",
				Path:              "/",
				Method:            "GET",
				Host:              host,
				Service:           TestrunnerName,
				Port:              TestRunnerPort,
				ConnectionTimeout: 10,
			}, SimpleHttpResponse, 1, 120*time.Second)
		})
	})

	Context("http ehco", func() {
		var (
			httpEcho *echoPod
		)
		BeforeEach(func() {
			var err error
			httpEcho, err = NewEchoHttp(namespace)
			Expect(err).NotTo(HaveOccurred())
			err = httpEcho.deploy(time.Minute)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err := httpEcho.Terminate()
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
			tcpEcho *echoPod
		)
		BeforeEach(func() {
			var err error
			tcpEcho, err = NewEchoTcp(namespace)
			Expect(err).NotTo(HaveOccurred())
			err = tcpEcho.deploy(time.Minute)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			err := tcpEcho.Terminate()
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

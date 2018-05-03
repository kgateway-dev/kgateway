package endpointswatcher_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers/local"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/pkg/log"
	"os"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"path/filepath"
)

func TestEndpointswatcher(t *testing.T) {
	if os.Getenv("RUN_CONSUL_TESTS") != "1" {
		log.Printf("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
		return
	}
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	log.DefaultOut = GinkgoWriter

	RegisterFailHandler(Fail)
	RunSpecs(t, "Endpointswatcher Suite")

}

var opts bootstrap.Options

// consul vars
var (
	consulFactory  *localhelpers.ConsulFactory
	consulInstance *localhelpers.ConsulInstance
	err            error
)

// kubernetes vars
var (
	namespace  string
	kubeClient kubernetes.Interface
)

var _ = BeforeSuite(func() {
	consulFactory, err = localhelpers.NewConsulFactory()
	helpers.Must(err)
	consulInstance, err = consulFactory.NewConsulInstance()
	helpers.Must(err)
	err = consulInstance.Run()
	helpers.Must(err)

	namespace = helpers.RandString(8)
	err = helpers.SetupKubeForTest(namespace)
	helpers.Must(err)

	opts = bootstrap.Options{
		KubeOptions: bootstrap.KubeOptions{
			Namespace:  namespace,
			KubeConfig: filepath.Join(os.Getenv("HOME"), ".kube", "config"),
		},
	}

	cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
	Expect(err).NotTo(HaveOccurred())

	// add a pod and service pointing to it
	kubeClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	consulInstance.Clean()
	consulFactory.Clean()
	helpers.TeardownKube(namespace)
})

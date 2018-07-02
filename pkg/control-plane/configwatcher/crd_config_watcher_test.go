package configwatcher

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/crd"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("KubeConfigWatcher", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		masterUrl, kubeconfigPath string
		namespace                 string
	)
	BeforeEach(func() {
		namespace = RandString(8)
		err := SetupKubeForTest(namespace)
		Must(err)
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		masterUrl = ""
	})
	AfterEach(func() {
		TeardownKube(namespace)
	})
	Describe("controller", func() {
		It("watches kube upstream crds", func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())

			storageClient, err := crd.NewStorage(cfg, namespace, time.Second)
			Expect(err).NotTo(HaveOccurred())

			watcher, err := NewConfigWatcher(storageClient)
			Must(err)
			go func() { watcher.Run(make(chan struct{})) }()

			upstream := NewTestUpstream1()
			created, err := storageClient.V1().Upstreams().Create(upstream)
			Expect(err).NotTo(HaveOccurred())

			// give controller time to register
			time.Sleep(time.Second * 2)

			select {
			case <-time.After(time.Second * 5):
				Expect(fmt.Errorf("expected to have received resource event before 5s")).NotTo(HaveOccurred())
			case cfg := <-watcher.Config():
				Expect(len(cfg.Upstreams)).To(Equal(1))
				Expect(cfg.Upstreams[0]).To(Equal(created))
				Expect(cfg.Upstreams[0].Spec).To(Equal(created.Spec))
			case err := <-watcher.Error():
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("watches kube virtual service crds", func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())

			storageClient, err := crd.NewStorage(cfg, namespace, time.Second)
			Expect(err).NotTo(HaveOccurred())

			watcher, err := NewConfigWatcher(storageClient)
			Must(err)
			go func() { watcher.Run(make(chan struct{})) }()

			virtualService := NewTestVirtualService("something", NewTestRoute1())
			created, err := storageClient.V1().VirtualServices().Create(virtualService)
			Expect(err).NotTo(HaveOccurred())

			// give controller time to register
			time.Sleep(time.Second * 2)

			select {
			case <-time.After(time.Second * 5):
				Expect(fmt.Errorf("expected to have received resource event before 5s")).NotTo(HaveOccurred())
			case cfg := <-watcher.Config():
				Expect(len(cfg.VirtualServices)).To(Equal(1))
				Expect(cfg.VirtualServices[0]).To(Equal(created))
				Expect(len(cfg.VirtualServices[0].Routes)).To(Equal(1))
				Expect(cfg.VirtualServices[0].Routes[0]).To(Equal(created.Routes[0]))
			case err := <-watcher.Error():
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("watches kube role crds", func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())

			storageClient, err := crd.NewStorage(cfg, namespace, time.Second)
			Expect(err).NotTo(HaveOccurred())

			watcher, err := NewConfigWatcher(storageClient)
			Must(err)
			go func() { watcher.Run(make(chan struct{})) }()

			role := NewTestRole("something", "foo")
			created, err := storageClient.V1().Roles().Create(role)
			Expect(err).NotTo(HaveOccurred())

			// give controller time to register
			time.Sleep(time.Second * 2)

			select {
			case <-time.After(time.Second * 5):
				Expect(fmt.Errorf("expected to have received resource event before 5s")).NotTo(HaveOccurred())
			case cfg := <-watcher.Config():
				Expect(len(cfg.Roles)).To(Equal(1))
				Expect(cfg.Roles[0]).To(Equal(created))
			case err := <-watcher.Error():
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})

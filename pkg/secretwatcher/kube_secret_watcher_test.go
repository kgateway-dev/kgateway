package secretwatcher

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"path/filepath"

	"fmt"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies/kube"
	. "github.com/solo-io/gloo/test/helpers"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("KubeSecretWatcher", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		masterUrl, kubeconfigPath string
		namespace                 string
		stop                      chan struct{}
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
		It("watches kube secrets", func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())

			secretClient, err := kube.NewSecretStorage(cfg, namespace, time.Second)
			Expect(err).NotTo(HaveOccurred())

			watcher, err := NewSecretWatcher(secretClient)
			Expect(err).NotTo(HaveOccurred())
			stop = make(chan struct{})

			go watcher.Run(stop)
			defer close(stop)

			// add a secret
			kubeClient, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "secret-",
					Namespace:    namespace,
				},
				Data: map[string][]byte{"username": []byte("me@example.com"), "password": []byte("foobar")},
			}

			createdSecret, err := kubeClient.CoreV1().Secrets(namespace).Create(secret)
			Expect(err).NotTo(HaveOccurred())

			// give controller time to register
			time.Sleep(time.Second * 2)

			go watcher.TrackSecrets([]string{createdSecret.Name})

			select {
			case <-time.After(time.Second * 5):
				Expect(fmt.Errorf("expected to have received resource event before 5s")).NotTo(HaveOccurred())
			case secrets := <-watcher.Secrets():
				Expect(len(secrets)).To(Equal(1))
				Expect(secrets[createdSecret.Name].Data["username"]).To(Equal("me@example.com"))
			case err := <-watcher.Error():
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})

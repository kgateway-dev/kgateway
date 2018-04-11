package kube_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"path/filepath"
	"time"

	. "github.com/solo-io/gloo/pkg/storage/dependencies/kube"
	restclient "k8s.io/client-go/rest"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/test/helpers"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func toStringMap(m map[string][]byte) map[string]string {
	sm := make(map[string]string)
	for k, v := range m {
		sm[k] = string(v)
	}
	return sm
}

var _ = Describe("Secret Storage Client", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		masterUrl, kubeconfigPath string
		namespace                 string
		syncFreq                  = time.Minute
		cfg                       *restclient.Config
		client                    dependencies.SecretStorage
		kube                      kubernetes.Interface
	)
	BeforeEach(func() {
		namespace = RandString(8)
		err := SetupKubeForTest(namespace)
		Must(err)
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		masterUrl = ""
		cfg, err = clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())
		client, err = NewSecretStorage(cfg, namespace, syncFreq)
		Expect(err).NotTo(HaveOccurred())
		kube, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		TeardownKube(namespace)
	})
	Describe("create", func() {
		It("creates the kube secret", func() {
			secret := &dependencies.Secret{
				Ref:  "good",
				Data: map[string]string{"hello": "goodbye"},
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			kubeSecret, err := kube.CoreV1().Secrets(namespace).Get(secret.Ref, v1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(kubeSecret.Data).To(HaveLen(1))
			Expect(toStringMap(kubeSecret.Data)).To(Equal(s.Data))
		})
		It("errors if the secret exists", func() {
			secret := &dependencies.Secret{
				Ref:  "good.secretname",
				Data: map[string]string{"hello": "goodbye"},
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			_, err = client.Create(secret)
			Expect(err).To(HaveOccurred())
		})
		It("creates the kube secret for a binary secret", func() {
			secretRef := "hi"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  secretRef,
				Data: data,
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			Expect(s.Data).To(Equal(data))
			kubeSecret, err := kube.CoreV1().Secrets(namespace).Get(secret.Ref, v1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(toStringMap(kubeSecret.Data)).To(Equal(data))
		})
		It("gets by name", func() {
			kubeSecretName := "good"
			key := "secretname"
			secretRef := kubeSecretName + "." + key
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  secretRef,
				Data: data,
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			Expect(s.Data).To(Equal(data))
			s2, err := client.Get(s.Ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(s2).To(Equal(s))
		})
		It("lists", func() {
			kubeSecretName := "good"
			key := "secretname"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  kubeSecretName + "1." + key,
				Data: data,
			}
			secret2 := &dependencies.Secret{
				Ref:  kubeSecretName + "2." + key,
				Data: data,
			}
			s1, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			s2, err := client.Create(secret2)
			Expect(err).NotTo(HaveOccurred())
			list, err := client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(ContainElement(s1))
			Expect(list).To(ContainElement(s2))
		})
		It("deletes", func() {
			kubeSecretName := "good"
			key := "secretname"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  kubeSecretName + "1." + key,
				Data: data,
			}
			s1, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			list, err := client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(ContainElement(s1))
			err = client.Delete(secret.Ref)
			Expect(err).NotTo(HaveOccurred())
			list, err = client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).NotTo(ContainElement(s1))
		})
		It("watches", func() {
			lists := make(chan []*dependencies.Secret, 3)
			stop := make(chan struct{})
			defer close(stop)
			errs := make(chan error)
			w, err := client.Watch(&dependencies.SecretEventHandlerFuncs{
				AddFunc: func(updatedList []*dependencies.Secret, obj *dependencies.Secret) {
					lists <- updatedList
				},
			})
			Expect(err).NotTo(HaveOccurred())
			go func() {
				w.Run(stop, errs)
			}()
			kubeSecretName := "good"
			key := "secretname"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  kubeSecretName + "1." + key,
				Data: data,
			}
			secret2 := &dependencies.Secret{
				Ref:  kubeSecretName + "2." + key,
				Data: data,
			}
			secret3 := &dependencies.Secret{
				Ref:  kubeSecretName + "3." + key,
				Data: data,
			}
			s1, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			s2, err := client.Create(secret2)
			Expect(err).NotTo(HaveOccurred())
			s3, err := client.Create(secret3)
			Expect(err).NotTo(HaveOccurred())
			Eventually(lists).Should(HaveLen(3))
			list1 := <-lists
			Expect(list1).To(HaveLen(1))
			Expect(list1).To(ContainElement(s1))
			list2 := <-lists
			Expect(list2).To(HaveLen(2))
			Expect(list2).To(ContainElement(s1))
			//Expect(list2).To(ContainElement(s2))
			list3 := <-lists
			Expect(list3).To(HaveLen(3))
			Expect(list3).To(ContainElement(s1))
			Expect(list3).To(ContainElement(s2))
			Expect(list3).To(ContainElement(s3))
		})
	})
})

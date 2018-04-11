package vault

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Secret Storage Client", func() {
	if os.Getenv("RUN_VAULT_TESTS") != "1" {
		log.Printf("This test creates vault resources and is disabled by default. To enable, set RUN_VAULT_TESTS=1 in your env.")
		return
	}

	var rootPath string
	var vault *api.Client
	var client dependencies.SecretStorage
	BeforeEach(func() {
		rootPath = "/secret/" + RandString(4)
		cfg := api.DefaultConfig()
		cfg.Address = "http://127.0.0.1:8200"
		c, err := api.NewClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		log.GreyPrintf("%s", vaultInstance.Token())
		c.SetToken(vaultInstance.Token())
		vault = c
		client = NewSecretStorage(c, rootPath, time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		vault.Logical().Delete(rootPath)
	})
	Describe("create", func() {
		It("creates the vault secret", func() {
			secret := &dependencies.Secret{
				Ref:  "good",
				Data: map[string]string{"hello": "goodbye"},
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			vaultSecret, err := vault.Logical().Read(rootPath + "/" + secret.Ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(vaultSecret.Data).To(HaveLen(1))
			Expect(vaultSecret.Data).To(Equal(toInterfaceMap(s.Data)))
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
		It("creates the vault secret for a binary secret", func() {
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
			vaultSecret, err := vault.Logical().Read(rootPath + "/" + secret.Ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(vaultSecret.Data).To(HaveLen(1))
			Expect(vaultSecret.Data).To(Equal(toInterfaceMap(data)))
		})
		It("gets by name", func() {
			vaultSecretName := "good"
			key := "secretname"
			secretRef := vaultSecretName + "." + key
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
			vaultSecretName := "good"
			key := "secretname"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  vaultSecretName + "1." + key,
				Data: data,
			}
			secret2 := &dependencies.Secret{
				Ref:  vaultSecretName + "2." + key,
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
			vaultSecretName := "good"
			key := "secretname"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  vaultSecretName + "1." + key,
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
			vaultSecretName := "good"
			key := "secretname"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  vaultSecretName + "1." + key,
				Data: data,
			}
			secret2 := &dependencies.Secret{
				Ref:  vaultSecretName + "2." + key,
				Data: data,
			}
			secret3 := &dependencies.Secret{
				Ref:  vaultSecretName + "3." + key,
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

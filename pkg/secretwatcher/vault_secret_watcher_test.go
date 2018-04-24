package secretwatcher

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"time"

	"os"

	"github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	vauiltsecrets "github.com/solo-io/gloo/pkg/storage/dependencies/vault"
	. "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

var _ = Describe("watching vault secrets", func() {
	if os.Getenv("RUN_VAULT_TESTS") != "1" {
		log.Printf("This test creates vault resources and is disabled by default. To enable, set RUN_VAULT_TESTS=1 in your env.")
		return
	}
	var (
		vaultFactory  *localhelpers.VaultFactory
		vaultInstance *localhelpers.VaultInstance
		err           error
		stop          chan struct{}
	)

	var _ = BeforeSuite(func() {
		vaultFactory, err = localhelpers.NewVaultFactory()
		Must(err)
		vaultInstance, err = vaultFactory.NewVaultInstance()
		Must(err)
		err = vaultInstance.Run()
		Must(err)
	})

	var _ = AfterSuite(func() {
		vaultInstance.Clean()
		vaultFactory.Clean()
	})

	var (
		rootPath string
		vault    *api.Client
		watch    Interface
	)
	BeforeEach(func() {
		rootPath = "/secret/" + RandString(4)
		cfg := api.DefaultConfig()
		cfg.Address = "http://127.0.0.1:8200"
		c, err := api.NewClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		c.SetToken(vaultInstance.Token())
		vault = c
		client := vauiltsecrets.NewSecretStorage(c, rootPath, time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		watch, err = NewSecretWatcher(client)
		Expect(err).NotTo(HaveOccurred())
		stop = make(chan struct{})
		go watch.Run(stop)
	})
	AfterEach(func() {
		close(stop)
		vault.Logical().Delete(rootPath)
	})

	Context("no secrets wanted", func() {
		It("doesnt send anything on any channel", func() {
			ref := "mysecret"
			_, err := vault.Logical().Write(rootPath+"/"+ref, map[string]interface{}{"some": "secret"})
			Expect(err).NotTo(HaveOccurred())
			select {
			case <-watch.Secrets():
				Fail("secretmap was received, expected timeout")
			case err := <-watch.Error():
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(time.Second * 1):
				// passed
			}
		})
	})
	Context("want secrets that the file doesn't contain", func() {
		It("sends nothing", func() {
			ref := "mysecret"
			_, err := vault.Logical().Write(rootPath+"/"+ref, map[string]interface{}{"some": "secret"})
			Expect(err).NotTo(HaveOccurred())
			go watch.TrackSecrets([]string{"this key really should not be in the secretmap"})
			select {
			case <-watch.Secrets():
				Fail("secretmap was received, expected error")
			case err := <-watch.Error():
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(time.Second * 1):
				// passed
			}
		})
	})
	Context("a valid secret is written to a file", func() {
		It("sends a corresponding secretmap on Secrets()", func() {
			secrets := map[string]interface{}{
				"password": "foo",
			}
			ref := "mysecret"
			stringSecrets := &dependencies.Secret{
				Ref: ref,
				Data: map[string]string{
					"password": "foo",
				},
			}
			_, err := vault.Logical().Write(rootPath+"/"+ref, secrets)
			Expect(err).NotTo(HaveOccurred())
			go watch.TrackSecrets([]string{ref})
			select {
			case parsedSecrets := <-watch.Secrets():
				Expect(parsedSecrets).To(Equal(SecretMap{ref: stringSecrets}))
			case err := <-watch.Error():
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(time.Second * 5):
				Fail("expected new secrets to be read in before 1s")
			}
		})
	})
})

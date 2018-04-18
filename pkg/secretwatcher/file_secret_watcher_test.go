package secretwatcher

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"path/filepath"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	filesecrets "github.com/solo-io/gloo/pkg/storage/dependencies/file"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("FileSecretWatcher", func() {
	var (
		dir   string
		file  string
		ref   string
		err   error
		watch Interface
		stop  chan struct{}
	)
	BeforeEach(func() {
		ref = "secrets.yml"
		dir, err = ioutil.TempDir("", "filesecrettest")
		Must(err)
		file = filepath.Join(dir, ref)
		secretClient, err := filesecrets.NewSecretStorage(dir, time.Millisecond)
		Must(err)
		watch, err = NewSecretWatcher(secretClient)
		Must(err)
		stop = make(chan struct{})

		go watch.Run(stop)
	})
	AfterEach(func() {
		close(stop)
		log.Debugf("removing " + dir)
		os.RemoveAll(dir)
	})
	Describe("watching file", func() {
		Context("no secrets wanted", func() {
			It("doesnt send anything on any channel", func() {
				missingSecrets := &dependencies.Secret{
					Ref:  ref,
					Data: map[string]string{"username": "me@example.com", "password": "foobar"},
				}
				data, err := yaml.Marshal(missingSecrets.Data)
				Expect(err).NotTo(HaveOccurred())
				err = ioutil.WriteFile(file, data, 0644)
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
		Context("valid secrets are written to the ref file", func() {
			It("sends a corresponding secretmap on Secrets()", func() {
				secret := &dependencies.Secret{
					Ref:  ref,
					Data: map[string]string{"username": "me@example.com", "password": "foobar"},
				}
				yml, err := yaml.Marshal(secret.Data)
				Must(err)
				err = ioutil.WriteFile(file, yml, 0644)
				Must(err)
				go watch.TrackSecrets([]string{ref})
				select {
				case parsedSecrets := <-watch.Secrets():
					Expect(parsedSecrets).To(Equal(SecretMap{
						ref: secret,
					}))
				case err := <-watch.Error():
					Expect(err).NotTo(HaveOccurred())
				case <-time.After(time.Second * 5):
					Fail("expected new secrets to be read in before 1s")
				}
			})
		})
	})
})

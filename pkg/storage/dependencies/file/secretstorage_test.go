package file_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/pkg/storage/dependencies/file"
	. "github.com/solo-io/gloo/test/helpers"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Client", func() {
	var (
		client dependencies.SecretStorage
		err    error
		dir    string
	)
	BeforeEach(func() {
		dir, err = ioutil.TempDir("", "")
		Must(err)
		client, err = NewSecretStorage(dir, time.Millisecond/2)
		Must(err)
	})
	AfterEach(func() { os.RemoveAll(dir) })
	Describe("create", func() {
		It("creates the secret on disk", func() {
			secret := &dependencies.Secret{
				Ref:  "filename",
				Data: map[string]string{"hello": "goodbye"},
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			b, err := ioutil.ReadFile(filepath.Join(dir, secret.Ref))
			Expect(err).NotTo(HaveOccurred())
			var data map[string]string
			err = yaml.Unmarshal(b, &data)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal(secret.Data))
		})
		It("errors if the secret exists", func() {
			secret := &dependencies.Secret{
				Ref:  "filename",
				Data: map[string]string{"hello": "goodbye"},
			}
			s, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).NotTo(BeNil())
			_, err = client.Create(secret)
			Expect(err).To(HaveOccurred())
		})
		It("gets by name", func() {
			fileRef := "filename"
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  fileRef,
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
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  "file1",
				Data: data,
			}
			file2 := &dependencies.Secret{
				Ref:  "file2",
				Data: data,
			}
			s1, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			s2, err := client.Create(file2)
			Expect(err).NotTo(HaveOccurred())
			list, err := client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(ContainElement(s1))
			Expect(list).To(ContainElement(s2))
		})
		It("deletes", func() {
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  "file1",
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
			data := map[string]string{"hello": "goodbye"}
			secret := &dependencies.Secret{
				Ref:  "file1",
				Data: data,
			}
			file2 := &dependencies.Secret{
				Ref:  "file2",
				Data: data,
			}
			file3 := &dependencies.Secret{
				Ref:  "file3",
				Data: data,
			}
			s1, err := client.Create(secret)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Millisecond * 100)
			s2, err := client.Create(file2)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Millisecond * 100)
			s3, err := client.Create(file3)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Millisecond * 100)
			list1 := <-lists
			Expect(list1).To(HaveLen(1))
			Expect(list1).To(ContainElement(s1))
			list2 := <-lists
			Expect(list2).To(HaveLen(2))
			Expect(list2).To(ContainElement(s1))
			Expect(list2).To(ContainElement(s2))
			list3 := <-lists
			Expect(list3).To(HaveLen(3))
			Expect(list3).To(ContainElement(s1))
			Expect(list3).To(ContainElement(s2))
			Expect(list3).To(ContainElement(s3))
		})
	})
})

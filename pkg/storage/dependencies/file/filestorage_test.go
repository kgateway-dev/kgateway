package file_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
	"unicode"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/pkg/storage/dependencies/file"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Client", func() {
	var (
		client dependencies.FileStorage
		err    error
		dir    string
	)
	BeforeEach(func() {
		dir, err = ioutil.TempDir("", "")
		Must(err)
		client, err = NewFileStorage(dir, time.Millisecond/2)
		Must(err)
	})
	AfterEach(func() { os.RemoveAll(dir) })
	Describe("create", func() {
		It("creates the file on disk", func() {
			file := &dependencies.File{
				Ref:      "filename",
				Contents: []byte("hello"),
			}
			f, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			b, err := ioutil.ReadFile(filepath.Join(dir, file.Ref))
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(Equal(file.Contents))
		})
		It("errors if the file exists", func() {
			file := &dependencies.File{
				Ref:      "filename",
				Contents: []byte("hello"),
			}
			f, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			_, err = client.Create(file)
			Expect(err).To(HaveOccurred())
		})
		It("gets by name", func() {
			fileRef := "filename"
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      fileRef,
				Contents: contents,
			}
			f, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			Expect(f.Contents).To(Equal(contents))
			f2, err := client.Get(f.Ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(f2).To(Equal(f))
		})
		It("lists", func() {
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      "file1",
				Contents: contents,
			}
			file2 := &dependencies.File{
				Ref:      "file2",
				Contents: contents,
			}
			f1, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			f2, err := client.Create(file2)
			Expect(err).NotTo(HaveOccurred())
			list, err := client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(ContainElement(f1))
			Expect(list).To(ContainElement(f2))
		})
		It("deletes", func() {
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      "file1",
				Contents: contents,
			}
			f1, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			list, err := client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(ContainElement(f1))
			err = client.Delete(file.Ref)
			Expect(err).NotTo(HaveOccurred())
			list, err = client.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(list).NotTo(ContainElement(f1))
		})
		It("watches", func() {
			lists := make(chan []*dependencies.File, 4)
			stop := make(chan struct{})
			defer close(stop)
			errs := make(chan error)
			w, err := client.Watch(&dependencies.FileEventHandlerFuncs{
				AddFunc: func(updatedList []*dependencies.File, obj *dependencies.File) {
					lists <- updatedList
				},
			})
			Expect(err).NotTo(HaveOccurred())
			go func() {
				w.Run(stop, errs)
			}()
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      "file1",
				Contents: contents,
			}
			file2 := &dependencies.File{
				Ref:      "file2",
				Contents: contents,
			}
			file3 := &dependencies.File{
				Ref:      "file3",
				Contents: contents,
			}
			f1, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Millisecond * 100)
			f2, err := client.Create(file2)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Millisecond * 100)
			f3, err := client.Create(file3)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Millisecond * 100)
			Eventually(lists).Should(HaveLen(4))
			// throw out the initial read
			<-lists
			list1 := <-lists
			Expect(list1).To(HaveLen(1))
			Expect(list1).To(ContainElement(f1))
			list2 := <-lists
			Expect(list2).To(HaveLen(2))
			Expect(list2).To(ContainElement(f1))
			Expect(list2).To(ContainElement(f2))
			list3 := <-lists
			Expect(list3).To(HaveLen(3))
			Expect(list3).To(ContainElement(f1))
			Expect(list3).To(ContainElement(f2))
			Expect(list3).To(ContainElement(f3))
		})
	})
})

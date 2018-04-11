package kube_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"path/filepath"
	"time"

	. "github.com/solo-io/gloo/pkg/storage/dependencies/kube"
	restclient "k8s.io/client-go/rest"

	"unicode"

	"encoding/base64"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/test/helpers"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("File Storage Client", func() {

	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		masterUrl, kubeconfigPath string
		namespace                 string
		syncFreq                  = time.Minute
		cfg                       *restclient.Config
		client                    dependencies.FileStorage
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
		client, err = NewFileStorage(cfg, namespace, syncFreq)
		Expect(err).NotTo(HaveOccurred())
		kube, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		TeardownKube(namespace)
	})
	Describe("create", func() {
		It("errors if the filename does not have a :", func() {
			file := &dependencies.File{
				Ref:      "badfilename",
				Contents: []byte{},
			}
			_, err := client.Create(file)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid file ref for kubernetes"))
		})
		It("creates the config map", func() {
			file := &dependencies.File{
				Ref:      "good:filename",
				Contents: []byte("hello"),
			}
			f, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			cm, err := kube.CoreV1().ConfigMaps(namespace).Get("good", v1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveLen(1))
			Expect(cm.Data).To(Equal(map[string]string{"filename": "hello"}))
		})
		It("errors if the file exists", func() {
			file := &dependencies.File{
				Ref:      "good:filename",
				Contents: []byte("hello"),
			}
			f, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			_, err = client.Create(file)
			Expect(err).To(HaveOccurred())
		})
		It("creates the config map for a binary file", func() {
			cmName := "good"
			key := "filename"
			fileRef := cmName + ":" + key
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      fileRef,
				Contents: contents,
			}
			f, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			Expect(f.Contents).To(Equal(contents))
			cm, err := kube.CoreV1().ConfigMaps(namespace).Get(cmName, v1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveLen(1))
			Expect(cm.BinaryData).To(HaveLen(0))
			Expect(cm.Data).To(Equal(map[string]string{key: base64.StdEncoding.EncodeToString(contents)}))
		})
		It("gets by name", func() {
			cmName := "good"
			key := "filename"
			fileRef := cmName + ":" + key
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
			cmName := "good"
			key := "filename"
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      cmName + "1:" + key,
				Contents: contents,
			}
			file2 := &dependencies.File{
				Ref:      cmName + "2:" + key,
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
			cmName := "good"
			key := "filename"
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      cmName + "1:" + key,
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
			lists := make(chan []*dependencies.File, 3)
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
			cmName := "good"
			key := "filename"
			contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
			file := &dependencies.File{
				Ref:      cmName + "1:" + key,
				Contents: contents,
			}
			file2 := &dependencies.File{
				Ref:      cmName + "2:" + key,
				Contents: contents,
			}
			file3 := &dependencies.File{
				Ref:      cmName + "3:" + key,
				Contents: contents,
			}
			f1, err := client.Create(file)
			Expect(err).NotTo(HaveOccurred())
			f2, err := client.Create(file2)
			Expect(err).NotTo(HaveOccurred())
			f3, err := client.Create(file3)
			Expect(err).NotTo(HaveOccurred())
			Eventually(lists).Should(HaveLen(3))
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

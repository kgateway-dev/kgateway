package configwatcher

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/file"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("FileConfigWatcher", func() {
	var (
		dir string
		err error
	)
	BeforeEach(func() {
		dir, err = ioutil.TempDir("", "filecachetest")
		Must(err)
	})
	AfterEach(func() {
		log.Debugf("removing " + dir)
		os.RemoveAll(dir)
	})
	Describe("controller", func() {
		It("watches gloo files", func() {
			storageClient, err := file.NewStorage(dir, time.Millisecond)
			Must(err)
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
	})
})

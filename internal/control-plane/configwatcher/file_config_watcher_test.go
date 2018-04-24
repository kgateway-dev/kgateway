//package configwatcher_test
//
//import (
//	"fmt"
//	"io/ioutil"
//	"os"
//	"path/filepath"
//	"sort"
//	"time"
//
//	"github.com/ghodss/yaml"
//	"github.com/gogo/protobuf/proto"
//	. "github.com/onsi/ginkgo"
//	. "github.com/onsi/gomega"
//	"github.com/pkg/errors"
//
//	"github.com/solo-io/gloo/pkg/storage/file"
//	. "github.com/solo-io/gloo/internal/configwatcher"
//	"github.com/solo-io/gloo/pkg/api/types/v1"
//	"github.com/solo-io/gloo/pkg/configwatcher"
//	"github.com/solo-io/gloo/pkg/log"
//	"github.com/solo-io/gloo/pkg/protoutil"
//	. "github.com/solo-io/gloo/test/helpers"
//)
//
//var _ = Describe("FileConfigWatcher", func() {
//	var (
//		dir                         string
//		err                         error
//		watch                       configwatcher.Interface
//		resourceDirs                = []string{"upstreams", "virtualservices"}
//		upstreamDir, virtualserviceDir string
//		upstreamFilename            = func(us *v1.Upstream) string {
//			return filepath.Join(upstreamDir, fmt.Sprintf("%v.yaml", us.Name))
//		}
//		virtualserviceFilename = func(vs *v1.VirtualService) string {
//			return filepath.Join(virtualserviceDir, fmt.Sprintf("%v.yaml", vs.Name))
//		}
//	)
//	BeforeEach(func() {
//		dir, err = ioutil.TempDir("", "filecachetest")
//		Must(err)
//		storageClient, err := file.NewStorage(dir, time.Millisecond)
//		Must(err)
//		watch, err = NewConfigWatcher(storageClient)
//		Must(err)
//		upstreamDir = filepath.Join(dir, "upstreams")
//		virtualserviceDir = filepath.Join(dir, "virtualservices")
//	})
//	AfterEach(func() {
//		log.Debugf("removing " + dir)
//		os.RemoveAll(dir)
//	})
//	Describe("init", func() {
//		It("creates the expected subdirs", func() {
//			files, err := ioutil.ReadDir(dir)
//			Expect(err).NotTo(HaveOccurred())
//			Expect(files).To(HaveLen(2))
//			var createdSubDirs []string
//			for _, file := range files {
//				Expect(file.IsDir()).To(BeTrue())
//				createdSubDirs = append(createdSubDirs, filepath.Base(file.Name()))
//			}
//			for _, expectedSubDir := range resourceDirs {
//				Expect(createdSubDirs).To(ContainElement(expectedSubDir))
//			}
//		})
//	})
//	Describe("watching directory", func() {
//		Context("valid configs are written to the correct directories", func() {
//			FIt("creates and updates configs for .yml or .yaml files found in the subdirs", func() {
//				cfg := NewTestConfig()
//				for _, us := range cfg.Upstreams {
//					err := writeConfigObjFile(us, upstreamFilename(us))
//					Expect(err).NotTo(HaveOccurred())
//				}
//				for _, vService := range cfg.VirtualServices {
//					err := writeConfigObjFile(vService, virtualserviceFilename(vService))
//					Expect(err).NotTo(HaveOccurred())
//				}
//				var expectedCfg v1.Config
//				data, err := protoutil.Marshal(cfg)
//				Expect(err).NotTo(HaveOccurred())
//				err = protoutil.Unmarshal(data, &expectedCfg)
//				Expect(err).NotTo(HaveOccurred())
//				var actualCfg *v1.Config
//				Eventually(func() (v1.Config, error) {
//					cfg, err := readConfig(watch)
//					sort.SliceStable(cfg.VirtualServices, func(i, j int) bool {
//						return cfg.VirtualServices[i].Name < cfg.VirtualServices[j].Name
//					})
//					sort.SliceStable(cfg.Upstreams, func(i, j int) bool {
//						return cfg.Upstreams[i].Name < cfg.Upstreams[j].Name
//					})
//					actualCfg = &cfg
//					log.Printf("%v", actualCfg.VirtualServices)
//					return cfg, err
//				}).Should(Equal(expectedCfg))
//			})
//		})
//		Context("an invalid config is written to a dir", func() {
//			It("sends an error on the Error() channel", func() {
//				invalidConfig := []byte("wdf112 1`12")
//				err = ioutil.WriteFile(filepath.Join(upstreamDir, "bad-upstream.yml"), invalidConfig, 0644)
//				Expect(err).NotTo(HaveOccurred())
//				select {
//				case <-watch.Config():
//					Fail("config was received, expected error")
//				case err := <-watch.Error():
//					Expect(err).To(HaveOccurred())
//				case <-time.After(time.Second * 1):
//					Fail("expected err to have occurred before 1s")
//				}
//			})
//		})
//	})
//})
//
//func writeConfigObjFile(v proto.Message, filename string) error {
//	jsn, err := protoutil.Marshal(v)
//	data, err := yaml.JSONToYAML(jsn)
//	if err != nil {
//		return err
//	}
//	return ioutil.WriteFile(filename, data, 0644)
//}
//
//var lastRead *v1.Config
//
//func readConfig(watch configwatcher.Interface) (v1.Config, error) {
//	select {
//	case cfg := <-watch.Config():
//		lastRead = cfg
//		return *cfg, nil
//	case err := <-watch.Error():
//		return v1.Config{}, err
//	case <-time.After(time.Second * 1):
//		if lastRead != nil {
//			return *lastRead, nil
//		}
//		return v1.Config{}, errors.New("expected new config to be read in before 1s")
//	}
//}

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

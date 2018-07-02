package filewatcher_test

import (
	"io/ioutil"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"encoding/json"

	"path/filepath"

	. "github.com/solo-io/gloo/pkg/control-plane/filewatcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	filestorage "github.com/solo-io/gloo/pkg/storage/dependencies/file"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("FileArtifactWatcher", func() {
	var (
		dir   string
		file  string
		ref   string
		err   error
		watch Interface
	)
	BeforeEach(func() {
		ref = "artifacts.yml"
		dir, err = ioutil.TempDir("", "fileartifacttest")
		Must(err)
		file = filepath.Join(dir, ref)
		store, err := filestorage.NewFileStorage(dir, time.Millisecond)
		Must(err)
		watch, err = NewFileWatcher(store)
		Must(err)
		go watch.Run(make(chan struct{}))
	})
	AfterEach(func() {
		log.Debugf("removing " + dir)
		os.RemoveAll(dir)
	})
	Describe("watching file", func() {
		Context("no artifacts wanted", func() {
			It("doesnt send anything on any channel", func() {
				missingArtifacts := map[string]map[string][]byte{"another-key": {"foo": []byte("bar"), "baz": []byte("qux")}}
				data, err := json.Marshal(missingArtifacts)
				Expect(err).NotTo(HaveOccurred())
				err = ioutil.WriteFile(file, data, 0644)
				Expect(err).NotTo(HaveOccurred())
				select {
				case f := <-watch.Files():
					log.Printf("%v", f)
					Fail("Files was received, expected timeout")
				case err := <-watch.Error():
					Expect(err).NotTo(HaveOccurred())
				case <-time.After(time.Second * 1):
					// passed
				}
			})
		})
		Context("valid artifacts are written to the ref file", func() {
			It("sends a corresponding Files on Files()", func() {

				data := []byte("this is the data")
				err = ioutil.WriteFile(file, data, 0644)
				Must(err)
				go watch.TrackFiles([]string{ref})
				select {
				case parsedArtifacts := <-watch.Files():
					Expect(parsedArtifacts).To(Equal(Files{
						ref: &dependencies.File{Ref: ref, Contents: data},
					}))
				case err := <-watch.Error():
					Expect(err).NotTo(HaveOccurred())
				case <-time.After(time.Second * 5):
					Fail("expected new artifacts to be read in before 1s")
				}
			})
		})
	})
})

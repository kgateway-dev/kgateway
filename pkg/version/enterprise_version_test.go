package version_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/spf13/afero"
)

var _ = Describe("LatestVersionFromRepo", func() {

	var fs afero.Fs
	var dir string
	var err error

	BeforeEach(func() {
		fs = afero.NewOsFs()
		dir, err = afero.TempDir(fs, "", "")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	It("returns the latest version", func() {
		fileString := `
apiVersion: v1
entries:
  gloo-ee:
  - apiVersion: v1
    version: 0.19.1
  - apiVersion: v1
    version: 0.10.1
  - apiVersion: v1
    version: 0.9.1
  - apiVersion: v1
    version: 0.8.1`
		tmpFile, err := afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = tmpFile.WriteString(fileString)
		Expect(err).To(BeNil())
		enterpriseVersion, err := version.LatestVersionFromRepo(tmpFile.Name(), version.GlooEE, true)
		Expect(err).To(BeNil())
		Expect(enterpriseVersion).To(Equal("0.19.1"))
	})

	It("ignores release candidate versions if stableOnly=true", func() {
		fileString := `
apiVersion: v1
entries:
  gloo-ee:
  - apiVersion: v1
    version: 1.0.0-rc2
  - apiVersion: v1
    version: 1.0.0-rc1
  - apiVersion: v1
    version: 0.20.1`
		tmpFile, err := afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = tmpFile.WriteString(fileString)
		Expect(err).To(BeNil())
		enterpriseVersion, err := version.LatestVersionFromRepo(tmpFile.Name(), version.GlooEE, true)
		Expect(err).To(BeNil())
		Expect(enterpriseVersion).To(Equal("0.20.1"))
	})

	It("doesn't ignore release candidate versions if stableOnly=false", func() {
		fileString := `
apiVersion: v1
entries:
  gloo-ee:
  - apiVersion: v1
    version: 1.0.0-rc2
  - apiVersion: v1
    version: 1.0.0-rc1
  - apiVersion: v1
    version: 0.20.1`
		tmpFile, err := afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = tmpFile.WriteString(fileString)
		Expect(err).To(BeNil())
		enterpriseVersion, err := version.LatestVersionFromRepo(tmpFile.Name(), version.GlooEE, false)
		Expect(err).To(BeNil())
		Expect(enterpriseVersion).To(Equal("1.0.0-rc2"))
	})

	It("works with versions > 1.0.0", func() {
		fileString := `
apiVersion: v1
entries:
  gloo-ee:
  - apiVersion: v1
    version: 1.2.0
  - apiVersion: v1
    version: 1.1.0
  - apiVersion: v1
    version: 1.0.0
  - apiVersion: v1
    version: 1.0.0-rc2
  - apiVersion: v1
    version: 1.0.0-rc1
  - apiVersion: v1
    version: 0.20.1`
		tmpFile, err := afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = tmpFile.WriteString(fileString)
		Expect(err).To(BeNil())
		enterpriseVersion, err := version.LatestVersionFromRepo(tmpFile.Name(), version.GlooEE, true)
		Expect(err).To(BeNil())
		Expect(enterpriseVersion).To(Equal("1.2.0"))
	})

	It("throws an error if the file is invalid", func() {
		fileString := `
apiVersion: v1
entries:
  gloo-ee:`
		tmpFile, err := afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = tmpFile.WriteString(fileString)
		Expect(err).To(BeNil())
		enterpriseVersion, err := version.LatestVersionFromRepo(tmpFile.Name(), version.GlooEE, false)
		Expect(enterpriseVersion).To(Equal(""))
		Expect(err).NotTo(BeNil())
	})
})

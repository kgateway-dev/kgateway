package version_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/testutils/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

type mockDirEntry struct {
	name string
}

// Name that returns a string like how a directory name / file name.
func (m mockDirEntry) Name() string {
	return m.name
}

var _ = Describe("upgrade utils unit tests", func() {
	baseEntries := []mockDirEntry{
		{"v1.7.0"}, {"v1.8.0-beta1"}, {"v1.7.1"},
	}
	Context("versions are returned as expected", func() {
		It("should return the last minor version", func() {
			entries := []mockDirEntry{{"v1.8.0-beta2"}}
			entries = append(entries, baseEntries...) // dont pollute baseEntries
			ver, curVer, err := version.ChangelogDirForLatestRelease(entries...)
			Expect(err).NotTo(HaveOccurred())
			Expect(ver.String()).To(Equal("v1.8.0-beta1"), fmt.Sprintf("%v", entries))
			Expect(curVer.String()).To(Equal("v1.8.0-beta2"), fmt.Sprintf("%v", entries))
		})
		It("should note that we are missing the last minor version", func() {

			ver, curVer, err := version.ChangelogDirForLatestRelease(baseEntries...)
			Expect(ver.String()).To(Equal("v1.7.1"), fmt.Sprintf("%v", baseEntries))
			Expect(err).To(HaveOccurred())
			Expect(curVer.String()).To(Equal("v1.8.0-beta1"), fmt.Sprintf("%v", baseEntries))
			Expect(err).To(Equal(version.FirstReleaseError))
		})
	})

})

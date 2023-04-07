package upgrade

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

type mockDirEntry struct {
	name string
}

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
			ver, err := filterFilesForLatestRelease(entries...)
			Expect(err).NotTo(HaveOccurred())
			Expect(ver.String()).To(Equal("1.8.0-beta2"))
		})
		It("should note that we are missing the last minor version", func() {

			ver, err := filterFilesForLatestRelease(baseEntries...)
			Expect(err).To(HaveOccurred())
			Expect(ver.String()).To(Equal("1.8.0-beta1"))
			Expect(err).To(Equal(FirstReleaseError))
		})
	})

	Context("Should never fail if you have internet"func(){
		It("should error or have a nil lastminor"func(){
			ver, err := GetUpgradeVersions(context.Background(), "gloo")
			
			belief := err != nil || ver == nil 
			Expect(belief).To(BeTrue())
		})
	})




})

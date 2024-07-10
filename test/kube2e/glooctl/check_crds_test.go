package glooctl_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Check-Crds", func() {

	It("validates correct CRDs", func() {
		if testHelper.ReleasedVersion != "" {
			_, err := GlooctlOut("check-crds", "--version", testHelper.ChartVersion())
			Expect(err).ToNot(HaveOccurred())
		} else {
			chartUri := filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
			_, err := GlooctlOut("check-crds", "--local-chart", chartUri)
			Expect(err).ToNot(HaveOccurred())
		}
	})
	It("fails with CRD mismatch", func() {
		_, err := GlooctlOut("check-crds", "--version", "1.9.0")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("One or more CRDs are out of date"))
	})

})

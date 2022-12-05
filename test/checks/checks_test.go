package checks

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/modfile"
)

var _ = Describe("Checks", func() {

	It("should used forked klog instead of klog", func() {
		// regular klog writes to disk, so make sure we used a forked version that doesn't write to
		// disk, which is a problem with hardened containers with root only file systems.

		allPackages, err := modfile.Parse()
		Expect(err).NotTo(HaveOccurred())

		replacedPackages := allPackages.Replace
		Expect(replacedPackages).NotTo(BeNil())

		var klogReplace modfile.ReplacedGoPackage
		for _, replacedGoPkg := range replacedPackages {
			if replacedGoPkg.Old.Path == "k8s.io/klog" {
				klogReplace = replacedGoPkg
				break
			}
		}

		Expect(klogReplace).NotTo(BeNil())
		Expect(klogReplace.New.Path).To(Equal("github.com/stefanprodan/klog"))
		Expect(klogReplace.New.Version).To(Equal("v0.0.0-20190418165334-9cbb78b20423"))
	})

})

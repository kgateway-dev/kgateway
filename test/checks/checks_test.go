package checks

import (
	"io/ioutil"
	"os/exec"
	"strings"

	"golang.org/x/mod/modfile"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Checks", func() {

	It("should used forked klog instead of klog", func() {
		// regular klog writes to disk, so make sure we used a forked version that doesn't write to
		// disk, which is a problem with hardened containers with root only file systems.

		gomod, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		gomodfile := strings.TrimSpace(string(gomod))
		data, err := ioutil.ReadFile(gomodfile)
		Expect(err).NotTo(HaveOccurred())
		modFile, err := modfile.Parse(gomodfile, data, nil)
		Expect(err).NotTo(HaveOccurred())

		var klogReplace *modfile.Replace
		for _, replace := range modFile.Replace {
			if replace.Old.Path == "k8s.io/klog" {
				klogReplace = replace
				break
			}
		}
		Expect(klogReplace).NotTo(BeNil())
		Expect(klogReplace.New.Path).To(Equal("github.com/stefanprodan/klog"))
		Expect(klogReplace.New.Version).To(Equal("v0.0.0-20190418165334-9cbb78b20423"))
	})

})

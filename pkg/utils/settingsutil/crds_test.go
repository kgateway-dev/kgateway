package settingsutil_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/settings"
)

var _ = Describe("Crds", func() {

	AfterEach(func() { os.Setenv("AUTO_CREATE_CRDS", "") })

	It("should not skip crd creation", func() {
		os.Setenv("AUTO_CREATE_CRDS", "1")
		Expect(GetSkipCrdCreation()).To(BeFalse())
	})

	It("should skip crd creation", func() {
		Expect(GetSkipCrdCreation()).To(BeTrue())
	})

})

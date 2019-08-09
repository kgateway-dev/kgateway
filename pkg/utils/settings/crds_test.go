package settings_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/settings"
)

var _ = Describe("Crds", func() {

	AfterEach(func() { os.Setenv("AUTO_CREATE_CRDS", "") })

	It("shoud not skip crd creation", func() {
		os.Setenv("AUTO_CREATE_CRDS", "1")
		Expect(SkipCrdCreation()).To(BeFalse())
	})

	It("shoud skip crd creation", func() {
		Expect(SkipCrdCreation()).To(BeTrue())
	})

})

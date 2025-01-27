package cliutil_test

import (
	. "github.com/kgateway-dev/kgateway/pkg/cliutil"
	"github.com/kgateway-dev/kgateway/pkg/cliutil/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetBoolInput", func() {
	It("correctly sets the input value", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("test msg [y/N]: ")
			c.SendLine("y")
			c.ExpectEOF()
		}, func() {
			var val bool
			err := GetBoolInput("test msg", &val)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeTrue())
		})
	})
})

package deployer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
)

var _ = Describe("Values Helpers", func() {
	Context("ComponentLogLevelsToString", func() {
		It("empty map should convert to empty string", func() {
			s, err := deployer.ComponentLogLevelsToString(map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeEmpty())
		})

		It("empty key should throw error", func() {
			_, err := deployer.ComponentLogLevelsToString(map[string]string{"": "val"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(deployer.ComponentLogLevelEmptyError("", "val").Error()))
		})

		It("empty value should throw error", func() {
			_, err := deployer.ComponentLogLevelsToString(map[string]string{"key": ""})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(deployer.ComponentLogLevelEmptyError("key", "").Error()))
		})

		It("should sort keys", func() {
			s, err := deployer.ComponentLogLevelsToString(map[string]string{
				"bbb": "val1",
				"cat": "val2",
				"a":   "val3",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(Equal("a:val3,bbb:val1,cat:val2"))
		})
	})
})

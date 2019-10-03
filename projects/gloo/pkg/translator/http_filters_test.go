package translator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("HttpFilters", func() {

	It("should have websocket upgrade config", func() {
		hcm := NewHttpConnectionManager(nil, nil, "rds")
		Expect(hcm.UpgradeConfigs).To(HaveLen(1))
		Expect(hcm.UpgradeConfigs[0].UpgradeType).To(Equal("websocket"))
	})

})

package ports_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gateway2/ports"
)

var _ = Describe("Ports", func() {

	It("should translate privileged port", func() {
		Expect(ports.TranslatePort(80)).To(Equal(uint16(8080)))
		Expect(ports.TranslatePort(443)).To(Equal(uint16(8443)))
		Expect(ports.TranslatePort(1023)).To(Equal(uint16(9023)))
	})
	It("should NOT translate unprivileged port", func() {
		Expect(ports.TranslatePort(8080)).To(Equal(uint16(8080)))
		Expect(ports.TranslatePort(8443)).To(Equal(uint16(8443)))
		Expect(ports.TranslatePort(1024)).To(Equal(uint16(1024)))
	})

})

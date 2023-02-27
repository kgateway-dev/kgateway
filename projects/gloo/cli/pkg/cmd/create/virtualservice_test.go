package create_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Virtualservice", func() {

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			err := testutils.Glooctl("create virtualservice")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})
	})

	It("can print as kube yaml in dry run", func() {
		out, err := testutils.GlooctlOut("create virtualservice kube --dry-run --name vs --domains foo.bar,baz.qux")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: vs
  namespace: gloo-system
spec:
  displayName: vs
  virtualHost:
    domains:
    - foo.bar
    - baz.qux
status: {}
`))
	})
	It("can print as solo-kit yaml in dry run", func() {
		out, err := testutils.GlooctlOut("create virtualservice kube --dry-run -oyaml --name vs --domains foo.bar,baz.qux")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`---
displayName: vs
metadata:
  name: vs
  namespace: gloo-system
virtualHost:
  domains:
  - foo.bar
  - baz.qux
`))
	})
})

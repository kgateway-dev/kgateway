package create_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Virtualservice", func() {
	It("can print as kube yaml", func() {
		out, err := testutils.GlooctlOut("create virtualservice kube --kubeyaml --name vs --domains foo.bar,baz.qux")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(`apiVersion: gateway.solo.io/v1
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
})

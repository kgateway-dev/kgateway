package add_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Root", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			err := testutils.Glooctl("add")
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(constants.SubcommandError))
		})
	})

	It("can print yaml in dry run", func() {
		out, err := testutils.GlooctlOut("add route --path-exact /all-pets --dest-name default-petstore-8080" +
			"--prefix-rewrite /api/pets --dry-run --name test")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: test
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - exact: /all-pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080--prefix-rewrite
            namespace: gloo-system
status: {}
`))
	})
})

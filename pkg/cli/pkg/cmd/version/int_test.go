package version_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/testutils"
)

var _ = Describe("version command", func() {

	It("will throw an error if namespace is set to an empty string", func() {
		err := testutils.Glooctl(`version -n `)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(version.NoNamespaceAllError))
	})

	It("will not error if flags are correct, whether kube_config is present or not", func() {
		err := testutils.Glooctl(`version`)
		Expect(err).NotTo(HaveOccurred())
	})

})

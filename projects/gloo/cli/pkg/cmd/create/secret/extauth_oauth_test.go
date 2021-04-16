package secret_test

import (
	"fmt"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
)

var _ = Describe("ExtauthOauth", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should create secret", func() {
		err := testutils.Glooctl("create secret oauth --name oauth --namespace gloo-system --client-secret 123")
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient().Read("gloo-system", "oauth", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetOauth()).To(Equal(&extauthpb.OauthSecret{ClientSecret: "123"}))
	})

	It("should error when no client secret provided", func() {
		err := testutils.Glooctl("create secret oauth --name oauth --namespace gloo-system")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("client-secret not provided"))
	})

	It("should error when no name provided", func() {
		err := testutils.Glooctl("create secret oauth --namespace gloo-system")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(argsutils.NameError))
	})

	It("can print the kube yaml in dry run", func() {
		out, err := testutils.GlooctlOut("create secret oauth --name oauth --namespace gloo-system --client-secret 123 --dry-run")
		Expect(err).NotTo(HaveOccurred())
		fmt.Print(out)
		Expect(out).To(ContainSubstring(`data:
  oauth: Y2xpZW50U2VjcmV0OiAiMTIzIgo=
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  name: oauth
  namespace: gloo-system
`))

	})

})

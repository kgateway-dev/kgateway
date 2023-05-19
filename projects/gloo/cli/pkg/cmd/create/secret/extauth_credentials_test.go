package secret_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("ExtauthApiKey", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	It("should create secret", func() {
		err := testutils.Glooctl("create secret authcredentials --name ldapservice --namespace gloo-system --username u --password p")
		Expect(err).NotTo(HaveOccurred())
		secret, err := helpers.MustSecretClient(ctx).Read("gloo-system", "ldapservice", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.GetCredentials().GetUsername()).To(Equal("u"))
		Expect(secret.GetCredentials().GetPassword()).To(Equal("p"))
	})
	It("should error when no user or password provided", func() {
		err := testutils.Glooctl("create secret authcredentials --name ldapservice --namespace gloo-system --password p")
		Expect(err).To(Equal(secret.MissingInputError))
		err = testutils.Glooctl("create secret authcredentials --name ldapservice --namespace gloo-system --username u")
		Expect(err).To(Equal(secret.MissingInputError))
	})
})

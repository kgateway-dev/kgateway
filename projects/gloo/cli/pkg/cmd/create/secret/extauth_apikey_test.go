package secret_test

import (
	"context"
	"fmt"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
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

	It("should create secret without labels", func() {
		err := testutils.Glooctl("create secret apikey --name user --namespace gloo-system --apikey secretApiKey")
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient(ctx).Read("gloo-system", "user", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetApiKey()).To(Equal(&extauthpb.ApiKey{
			ApiKey: "secretApiKey",
		}))
	})

	It("should create secret with labels", func() {
		err := testutils.Glooctl("create secret apikey --name user --namespace gloo-system --apikey secretApiKey --apikey-labels k1=v1,k2=v2")
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient(ctx).Read("gloo-system", "user", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetApiKey()).To(Equal(&extauthpb.ApiKey{
			ApiKey: "secretApiKey",
		}))
		Expect(secret.Metadata.Labels).To(Equal(
			map[string]string{
				"k1": "v1",
				"k2": "v2",
			}))
	})

	It("should error when no apikey provided", func() {
		err := testutils.Glooctl("create secret apikey --name user --namespace gloo-system")
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(secret.MissingApiKeyError))
	})

	It("should error when no name provided", func() {
		err := testutils.Glooctl("create secret apikey --namespace gloo-system")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(argsutils.NameError))
	})

	It("can print the kube yaml in dry run", func() {
		out, err := testutils.GlooctlOut("create secret apikey --name user --namespace gloo-system --apikey secretApiKey --apikey-labels k1=v1,k2=v2 --dry-run")
		Expect(err).NotTo(HaveOccurred())
		fmt.Print(out)
		Expect(out).To(ContainSubstring(`metadata:
  creationTimestamp: null
  labels:
    k1: v1
    k2: v2
  name: user
  namespace: gloo-system
stringData:
  api-key: secretApiKey
type: extauth.solo.io/apikey
`))
	})

})

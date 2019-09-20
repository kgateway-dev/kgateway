package secret_test

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	pluginutils "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("ExtauthApiKey", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should create secret without labels", func() {
		err := testutils.Glooctl("create secret apikey --name user --namespace gloo-system --apikey secretApiKey")
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient().Read("gloo-system", "user", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		var extension extauthpb.ApiKeySecret
		err = pluginutils.ExtensionToProto(secret.GetExtension(), constants.ExtAuthExtensionName, &extension)
		Expect(err).NotTo(HaveOccurred())

		Expect(extension).To(Equal(extauthpb.ApiKeySecret{
			ApiKey: "secretApiKey",
			Labels: []string{},
		}))
	})

	It("should create secret with labels", func() {
		err := testutils.Glooctl("create secret apikey --name user --namespace gloo-system --apikey secretApiKey --apikey-labels k1=v1,k2=v2")
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient().Read("gloo-system", "user", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		var extension extauthpb.ApiKeySecret
		err = pluginutils.ExtensionToProto(secret.GetExtension(), constants.ExtAuthExtensionName, &extension)
		Expect(err).NotTo(HaveOccurred())

		Expect(extension).To(Equal(extauthpb.ApiKeySecret{
			ApiKey: "secretApiKey",
			Labels: []string{"k1=v1", "k2=v2"},
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
		Expect(out).To(Equal(`data:
  extension: Y29uZmlnOgogIGFwaV9rZXk6IHNlY3JldEFwaUtleQogIGxhYmVsczoKICAtIGsxPXYxCiAgLSBrMj12Mgo=
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  labels:
    k1: v1
    k2: v2
  name: user
  namespace: gloo-system
`))
	})

})

package secret_test

import (
	"context"
	"fmt"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("EncryptionKey", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	const (
		encryptionKey        = "this1is2an3encryption4key5exampl"
		encryptionKeyBase64  = "dGhpczFpczJhbjNlbmNyeXB0aW9uNGtleTVleGFtcGw="
		invalidEncryptionKey = "this is an encryption key examp"
		secretName           = "my-encryption-key"
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	It("should create an encryption secret", func() {
		err := testutils.Glooctl(fmt.Sprintf("create secret encryptionkey --name %s --key %s", secretName, encryptionKey))
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient(ctx).Read("gloo-system", secretName, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetEncryption()).To(Equal(&gloov1.EncryptionKeySecret{
			Key: encryptionKey,
		}))
	})

	It("should error when no name provided", func() {
		err := testutils.Glooctl(fmt.Sprintf("create secret encryptionkey --key %s", encryptionKey))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(argsutils.NameError))
	})

	It("should error when invalid key is used", func() {
		err := testutils.Glooctl(fmt.Sprintf("create secret encryptionkey --name %s --key %s", secretName, invalidEncryptionKey))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal(secret.KeyIsNotValidLength.Error()))
	})

	It("can print the kube yaml in dry run", func() {
		out, err := testutils.GlooctlOut(fmt.Sprintf("create secret encryptionkey --name %s --namespace gloo-system --key %s --dry-run", secretName, encryptionKey))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`data:
  key: ` + encryptionKeyBase64 + `
metadata:
  creationTimestamp: null
  name: ` + secretName + `
  namespace: gloo-system
type: gloo.solo.io.EncryptionKeySecret
`))
	})

})

package secret_test

import (
	"fmt"

	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

var _ = Describe("Secret", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	It("should create aws secret", func() {
		err := testutils.Glooctl("create secret aws --name test --namespace gloo-system --access-key foo --secret-key bar")
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient().Read("gloo-system", "test", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		aws := v1.AwsSecret{
			AccessKey: "foo",
			SecretKey: "bar",
		}
		Expect(*secret.GetAws()).To(Equal(aws))
	})

	It("should create tls secret", func() {
		rootca := mustWriteTestFile("foo")
		defer os.Remove(rootca)
		privatekey := mustWriteTestFile("bar")
		defer os.Remove(privatekey)
		certchain := mustWriteTestFile("baz")
		defer os.Remove(certchain)
		args := fmt.Sprintf(
			"create secret tls test --name test --namespace gloo-system --rootca %s --privatekey %s --certchain %s",
			rootca,
			privatekey,
			certchain)

		err := testutils.Glooctl(args)
		Expect(err).NotTo(HaveOccurred())

		secret, err := helpers.MustSecretClient().Read("gloo-system", "test", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		tls := v1.TlsSecret{
			RootCa:     "foo",
			PrivateKey: "bar",
			CertChain:  "baz",
		}
		Expect(*secret.GetTls()).To(Equal(tls))
	})

})

func mustWriteTestFile(contents string) string {
	tmpFile, err := ioutil.TempFile("", "test-")

	if err != nil {
		log.Fatalf("Failed to create test file: %v", err)
	}

	text := []byte(contents)
	if _, err = tmpFile.Write(text); err != nil {
		log.Fatalf("Failed to write to test file: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		log.Fatalf("Failed to write to test file: %v", err)
	}

	return tmpFile.Name()
}

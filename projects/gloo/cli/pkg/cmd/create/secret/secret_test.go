package secret_test

import (
	"fmt"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	"io/ioutil"

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

	Context("AWS", func() {
		It("should error if no name provided", func() {
			err := testutils.Glooctl("create secret aws")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		shouldWork := func(command, namespace string) {
			err := testutils.Glooctl(command)
			Expect(err).NotTo(HaveOccurred())

			secret, err := helpers.MustSecretClient().Read(namespace, "test", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			aws := v1.AwsSecret{
				AccessKey: "foo",
				SecretKey: "bar",
			}
			Expect(*secret.GetAws()).To(Equal(aws))
		}

		It("should work", func() {
			shouldWork("create secret aws --name test --access-key foo --secret-key bar", "gloo-system")
		})

		It("can print the kube yaml", func() {
			out, err := testutils.GlooctlOut("create secret aws --kubeyaml --name test --access-key foo --secret-key bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(`data:
  aws: YWNjZXNzS2V5OiBmb28Kc2VjcmV0S2V5OiBiYXIK
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  name: test
  namespace: gloo-system
`))
		})

		It("should work as subcommand", func() {
			shouldWork("create secret aws test --access-key foo --secret-key bar", "gloo-system")
		})

		It("should work in custom namespace", func() {
			shouldWork("create secret aws test --namespace custom --access-key foo --secret-key bar", "custom")
		})
	})

	Context("Azure", func() {
		It("should error if no name provided", func() {
			err := testutils.Glooctl("create secret azure")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		shouldWork := func(command, namespace string) {
			err := testutils.Glooctl(command)
			Expect(err).NotTo(HaveOccurred())

			secret, err := helpers.MustSecretClient().Read(namespace, "test", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			azure := v1.AzureSecret{
				ApiKeys: map[string]string{
					"foo":  "bar",
					"gloo": "baz",
				},
			}
			Expect(*secret.GetAzure()).To(Equal(azure))
		}

		It("should work", func() {
			shouldWork("create secret azure --name test --api-keys foo=bar,gloo=baz", "gloo-system")
		})

		It("can print the kube yaml", func() {
			out, err := testutils.GlooctlOut("create secret azure --kubeyaml --name test --name test --api-keys foo=bar,gloo=baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(`data:
  azure: YXBpS2V5czoKICBmb286IGJhcgogIGdsb286IGJhego=
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  name: test
  namespace: gloo-system
`))
		})

		It("should work as subcommand", func() {
			shouldWork("create secret azure test --api-keys foo=bar,gloo=baz", "gloo-system")
		})

		It("should work with custom namespace", func() {
			shouldWork("create secret azure test --namespace custom --api-keys foo=bar,gloo=baz", "custom")
		})
	})

	Context("TLS", func() {
		It("should error if no name provided", func() {
			err := testutils.Glooctl("create secret tls")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		It("should work", func() {
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
		It("can print the kube yaml", func() {
			rootca := mustWriteTestFile("foo")
			defer os.Remove(rootca)
			privatekey := mustWriteTestFile("bar")
			defer os.Remove(privatekey)
			certchain := mustWriteTestFile("baz")
			defer os.Remove(certchain)
			args := fmt.Sprintf(
				"create secret tls test --kubeyaml --name test --namespace gloo-system --rootca %s --privatekey %s --certchain %s",
				rootca,
				privatekey,
				certchain)

			out, err := testutils.GlooctlOut(args)
			Expect(err).NotTo(HaveOccurred())

			Expect(out).To(Equal(`data:
  tls: Y2VydENoYWluOiBiYXoKcHJpdmF0ZUtleTogYmFyCnJvb3RDYTogZm9vCg==
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  name: test
  namespace: gloo-system
`))
		})
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

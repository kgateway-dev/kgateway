package syncutil_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Log Redacter", func() {
	var (
		secretName      = "my-test-secret"
		secretNamespace = "my-secret-namespace"
		privateKey      = "RSA PRIVATE KEY CONTENT"

		noSecretsSnapshot = &v1.SetupSnapshot{
			Settings: []*v1.Settings{{
				Metadata: core.Metadata{
					Name:      "settings",
					Namespace: "ns",
				},
			}},
		}
		snapshotWithSecrets = &v1.ApiSnapshot{
			Endpoints: []*v1.Endpoint{{
				Metadata: core.Metadata{
					Name:      "endpoint",
					Namespace: "ns",
				},
			}},
			Secrets: []*v1.Secret{{
				Kind: &v1.Secret_Tls{Tls: &v1.TlsSecret{
					PrivateKey: privateKey,
				}},
				Metadata: core.Metadata{
					Name:      secretName,
					Namespace: secretNamespace,
				},
			}},
		}
	)

	It("does not redact anything when no secrets", func() {
		Expect(syncutil.StringifySnapshot(noSecretsSnapshot)).NotTo(ContainSubstring(syncutil.Redacted))
	})

	It("contains redacted content when secrets are present", func() {
		s := syncutil.StringifySnapshot(snapshotWithSecrets)

		Expect(s).To(ContainSubstring(syncutil.Redacted))
		Expect(s).NotTo(ContainSubstring(privateKey))
	})
})

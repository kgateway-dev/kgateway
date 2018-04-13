package azure

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

var _ = Describe("AzurePublicProfile", func() {
	It("gets the user password from the xml in the secret", func() {
		ref := "secret_ref"
		us := &v1.Upstream{
			Name: "whatever",
			Metadata: &v1.Metadata{
				Annotations: map[string]string{annotationKey: ref},
			},
		}
		secrets := secretwatcher.SecretMap{ref: {Ref: ref, Data: map[string]string{
			publishProfileKey: profileStringTemplate,
		}}}
		username, pass, err := getUserCredentials(us, secrets)
		Expect(err).NotTo(HaveOccurred())
		Expect(pass).To(Equal("{{ .Password }}"))
		Expect(username).To(Equal("${{ .AppName }}"))
	})
})

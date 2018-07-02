package azure

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("GetAzureFuncs", func() {
	It("gets the token", func() {
		targetFunctionName := os.Getenv("AZURE_FUNCTION_NAME")
		if targetFunctionName == "" {
			Skip("must set AZURE_FUNCTION_NAME to run this test")
		}
		targetFunctionAppName := os.Getenv("AZURE_FUNCTION_APP")
		if targetFunctionAppName == "" {
			Skip("must set AZURE_FUNCTION_APP to run this test")
		}
		ref := "secret_ref"
		us := &v1.Upstream{
			Name: "whatever",
			Spec: azure.EncodeUpstreamSpec(azure.UpstreamSpec{
				FunctionAppName: targetFunctionAppName,
			}),
			Metadata: &v1.Metadata{
				Annotations: map[string]string{annotationKey: ref},
			},
		}
		secrets := secretwatcher.SecretMap{ref: {Ref: ref, Data: map[string]string{
			publishProfileKey: helpers.AzureProfileString(),
		}}}

		var funcs []*v1.Function
		var secret *dependencies.Secret
		var err error

		Eventually(func() []*v1.Function {
			funcs, secret, err = GetFuncsAndSecret(us, secrets)
			return funcs
		}, "1m", "5s").Should(Not(BeEmpty()))

		var testPassed bool
		for _, fn := range funcs {
			if fn.Name != targetFunctionName {
				continue
			}
			azFnSpec, err := azure.DecodeFunctionSpec(fn.Spec)
			Expect(err).To(BeNil())
			Expect(azFnSpec.AuthLevel).To(Equal(authLevelFunction))
			Expect(secret.Data[azFnSpec.FunctionName]).NotTo(BeEmpty())
			testPassed = true
		}
		if !testPassed {
			Fail("did not find target function " + targetFunctionName)
		}
	})
})

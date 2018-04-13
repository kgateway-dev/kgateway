package azure

import (
	"os"
	"text/template"

	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

var _ = Describe("GetAzureFuncs", func() {
	It("gets the token", func() {
		targetFunctionName := os.Getenv("AZURE_FUNCTION_NAME")
		if targetFunctionName == "" {
			Skip("must set AZURE_FUNCTION_NAME to run this test")
		}
		ref := "secret_ref"
		us := &v1.Upstream{
			Name: "whatever",
			Spec: azure.EncodeUpstreamSpec(azure.UpstreamSpec{
				FunctionAppName: os.Getenv("AZURE_FUNCTION_APP"),
			}),
			Metadata: &v1.Metadata{
				Annotations: map[string]string{annotationKey: ref},
			},
		}
		secrets := secretwatcher.SecretMap{ref: {Ref: ref, Data: map[string]string{
			publishProfileKey: profileString(),
		}}}
		funcs, secret, err := GetFuncsAndSecret(us, secrets)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(funcs)).To(BeNumerically(">=", 1))
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

func profileString() string {
	data := struct {
		AppName  string
		Password string
	}{
		AppName:  os.Getenv("AZURE_FUNCTION_APP"),
		Password: os.Getenv("AZURE_PROFILE_PASSWORD"),
	}
	if data.AppName == "" || data.Password == "" {
		Skip("must set AZURE_FUNCTION_APP and AZURE_PROFILE_PASSWORD to run this test")
	}
	tmpl, err := template.New("ProfileString").Parse(profileStringTemplate)
	if err != nil {
		panic(errors.Wrap(err, "parsing template"))
	}
	buf := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(buf, "ProfileString", data); err != nil {
		panic(errors.Wrap(err, "executing template"))
	}

	return buf.String()
}

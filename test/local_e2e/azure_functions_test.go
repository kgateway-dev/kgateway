package local_e2e

import (
	"errors"
	"net/http"
	"os"

	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/protoutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const AzureSecretRef = "azure-secret"

const azureSkipMsg = ". Run like so: AZURE_FUNCTION_APP=<function app name> AZURE_UPPERCASE=<function name> AZURE_API_KEY=<api key> ginkgo"

func NewAzureUpstream() *v1.Upstream {
	azureFunctionApp := os.Getenv("AZURE_FUNCTION_APP")
	if azureFunctionApp == "" {
		Skip("no Azure Function App project, test cannot continue" + azureSkipMsg)
	}

	azureFunction := os.Getenv("AZURE_UPPERCASE")
	if azureFunction == "" {
		Skip("no Azure uppercase function, test cannot continue" + azureSkipMsg)
	}

	serviceSpec := azure.UpstreamSpec{
		FunctionAppName: azureFunctionApp,
		SecretRef:       AzureSecretRef,
	}
	v1Spec, err := protoutil.MarshalStruct(serviceSpec)
	if err != nil {
		panic(err)
	}

	funcSpec := azure.FunctionSpec{
		FunctionName: azureFunction,
		AuthLevel:    "anonymous",
	}
	v1FSpec, err := protoutil.MarshalStruct(funcSpec)
	if err != nil {
		panic(err)
	}

	f := &v1.Function{
		Name: azureFunction,
		Spec: v1FSpec,
	}
	annotations := make(map[string]string)
	annotations["gloo.solo.io/azure_secret_ref"] = AzureSecretRef
	u := &v1.Upstream{
		Name:      "local", // TODO: randomize
		Type:      azure.UpstreamTypeAzure,
		Spec:      v1Spec,
		Functions: []*v1.Function{f},
		Metadata: &v1.Metadata{
			Annotations: annotations,
		},
	}

	return u
}

var _ = Describe("Azure Functions", func() {
	It("Should work with an existing function", func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()

		au := NewAzureUpstream()

		err = glooInstance.AddUpstream(au)
		Expect(err).NotTo(HaveOccurred())

		v := &v1.VirtualHost{
			Name: "default",
			Routes: []*v1.Route{{
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Function{
						Function: &v1.FunctionDestination{
							UpstreamName: au.Name,
							FunctionName: au.Functions[0].Name,
						},
					},
				},
			}},
		}

		err = glooInstance.AddVhost(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`{"message": "solo.io"}`)

		// wait for envoy to start receiving request
		Eventually(func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			resp, err := http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/json", &buf)
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 {
				return errors.New("request is not 200 ok")
			}

			return nil
		}, 60, 1).Should(BeNil())

		// send a request with a body
		var buf bytes.Buffer
		buf.Write(body)
		resp, err := http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/json", &buf)
		Expect(err).NotTo(HaveOccurred())

		var rbody []byte
		if resp.Body != nil {
			rbody, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
		}

		Expect("SOLO.IO").To(Equal(string(rbody)))

	})

})

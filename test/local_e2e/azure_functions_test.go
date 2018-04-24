package local_e2e

import (
	"errors"
	"io"
	"net/http"
	"os"

	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/protoutil"

	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
)

const publishProfileSecretRef = "azure-publish-profile"

const azureSkipMsg = ". Run like so: AZURE_FUNCTION_APP=<function app name> AZURE_UPPERCASE=<function name> AZURE_PROFILE_PASSWORD=<azure publish profile password> ginkgo"

func NewAzureUpstream() *v1.Upstream {
	azureFunctionApp := os.Getenv("AZURE_FUNCTION_APP")
	if azureFunctionApp == "" {
		Skip("no Azure Function App project, test cannot continue" + azureSkipMsg)
	}

	upstreamSpec := azure.UpstreamSpec{
		FunctionAppName: azureFunctionApp,
	}
	v1Spec, err := protoutil.MarshalStruct(upstreamSpec)
	if err != nil {
		panic(err)
	}

	annotations := make(map[string]string)
	annotations["gloo.solo.io/azure_publish_profile"] = publishProfileSecretRef
	u := &v1.Upstream{
		Name: "local", // TODO: randomize
		Type: azure.UpstreamTypeAzure,
		Spec: v1Spec,
		Metadata: &v1.Metadata{
			Annotations: annotations,
		},
	}

	return u
}

var _ = Describe("Azure Functions", func() {
	var (
		au        *v1.Upstream
		envoyPort uint32
	)
	BeforeEach(func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = functionDiscoveryInstance.Run(glooInstance.ConfigDir())
		Expect(err).NotTo(HaveOccurred())

		glooInstance.AddSecret(publishProfileSecretRef, map[string]string{
			"publish_profile": helpers.AzureProfileString(),
		})

		envoyPort = glooInstance.EnvoyPort()

		au = NewAzureUpstream()

		err = glooInstance.AddUpstream(au)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		functionDiscoveryInstance.Clean()
		glooInstance.Clean()
		envoyInstance.Clean()
	})
	It("Should work with an existing function", func() {
		// for function discovery
		azureFunction := os.Getenv("AZURE_UPPERCASE")
		if azureFunction == "" {
			Skip("no Azure uppercase function, test cannot continue" + azureSkipMsg)
		}

		funcSpec := azure.FunctionSpec{
			FunctionName: azureFunction,
			AuthLevel:    "function",
		}

		v1FSpec, err := protoutil.MarshalStruct(funcSpec)
		helpers.Must(err)

		expectedFunction := &v1.Function{
			Name: azureFunction,
			Spec: v1FSpec,
		}

		Eventually(func() ([]*v1.Function, error) {
			us, err := glooInstance.GetUpstream(au.Name)
			if err != nil {
				return nil, err
			}
			return us.Functions, nil
		}, time.Second*10).Should(ContainElement(expectedFunction))

		v := &v1.VirtualService{
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
							FunctionName: expectedFunction.Name,
						},
					},
				},
			}},
		}

		err = glooInstance.AddvService(v)
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
				fmt.Fprintf(GinkgoWriter, "resp is: %d\n", resp.StatusCode)
				io.Copy(GinkgoWriter, resp.Body)
				fmt.Fprintln(GinkgoWriter)
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
	It("calls a function with function level auth", func() {
		// for function discovery
		azureFunction := os.Getenv("AZURE_FUNCTION_NAME")
		if azureFunction == "" {
			Skip("no Azure auth function, test cannot continue. Specify with AZURE_FUNCTION_NAME")
		}

		funcSpec := azure.FunctionSpec{
			FunctionName: azureFunction,
			AuthLevel:    "function",
		}

		v1FSpec, err := protoutil.MarshalStruct(funcSpec)
		helpers.Must(err)

		expectedFunction := &v1.Function{
			Name: azureFunction,
			Spec: v1FSpec,
		}

		Eventually(func() ([]*v1.Function, error) {
			us, err := glooInstance.GetUpstream(au.Name)
			if err != nil {
				return nil, err
			}
			return us.Functions, nil
		}, time.Second*10).Should(ContainElement(expectedFunction))

		v := &v1.VirtualService{
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
							FunctionName: expectedFunction.Name,
						},
					},
				},
			}},
		}

		err = glooInstance.AddvService(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte(`{"name": "solo.io"}`)

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
				fmt.Fprintf(GinkgoWriter, "resp is: %d\n", resp.StatusCode)
				io.Copy(GinkgoWriter, resp.Body)
				fmt.Fprintln(GinkgoWriter)
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

		Expect("\"Hello solo.io\"").To(Equal(string(rbody)))
	})

})

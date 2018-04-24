package local_e2e

import (
	"errors"
	"net/http"
	"os"

	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/google"
	"github.com/solo-io/gloo/pkg/protoutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const GoogleSecretRef = "google-secret"

const skipMsg = ". Run like so: GOOGLE_PROJECT=rare-basis-116812 GOOGLE_REGION=us-central1 GOOGLE_UPPERCASE=function-name GOOGLE_SECRET_PATH=/path/to-key/key-fffffffffff.json ginkgo"

func NewGoogleUpstream() *v1.Upstream {

	googleProject := os.Getenv("GOOGLE_PROJECT")
	if googleProject == "" {
		Skip("no google project, test cannot continue" + skipMsg)
	}

	googleRegion := os.Getenv("GOOGLE_REGION")
	if googleRegion == "" {
		Skip("no google region, test cannot continue" + skipMsg)
	}

	googleFunction := os.Getenv("GOOGLE_UPPERCASE")
	if googleFunction == "" {
		Skip("no google uppercase function, test cannot continue" + skipMsg)
	}

	serviceSpec := gfunc.UpstreamSpec{
		Region:    googleRegion,
		ProjectId: googleProject,
	}
	v1Spec, err := protoutil.MarshalStruct(serviceSpec)
	if err != nil {
		panic(err)
	}

	funcSpec, err := gfunc.NewFuncFromUrl(fmt.Sprintf("https://%s-%s.cloudfunctions.net/%s", googleProject, googleProject, googleFunction))
	if err != nil {
		panic(err)
	}

	v1FSpec, err := protoutil.MarshalStruct(funcSpec)
	if err != nil {
		panic(err)
	}

	f := &v1.Function{
		Name: fmt.Sprintf("projects/%s/locations/%s/functions/%s", googleProject, googleRegion, googleFunction),
		Spec: v1FSpec,
	}
	annotations := make(map[string]string)
	annotations["gloo.solo.io/google_secret_ref"] = GoogleSecretRef
	u := &v1.Upstream{
		Name:      "local", // TODO: randomize
		Type:      gfunc.UpstreamTypeGoogle,
		Spec:      v1Spec,
		Functions: []*v1.Function{f},
		Metadata: &v1.Metadata{
			Annotations: annotations,
		},
	}

	return u
}

var _ = Describe("Google functions", func() {
	It("should discover functions and proxy", func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		secret := make(map[string]string)
		secretFilePath := os.Getenv("GOOGLE_SECRET_PATH")
		if secretFilePath == "" {
			Skip("no google secrets, test cannot continue" + skipMsg)
		}
		secretFile, err := ioutil.ReadFile(secretFilePath)
		Expect(err).NotTo(HaveOccurred())
		secret["json_key_file"] = string(secretFile)
		glooInstance.AddSecret(GoogleSecretRef, secret)

		err = functionDiscoveryInstance.Run(glooInstance.ConfigDir())
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()

		gu := NewGoogleUpstream()

		funcname := gu.Functions[0].Name

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
							UpstreamName: gu.Name,
							FunctionName: funcname,
						},
					},
				},
			}},
		}

		// remove functions as we test discovery
		gu.Functions = nil
		err = glooInstance.AddUpstream(gu)
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.AddvService(v)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() []*v1.Function {
			us, err := glooInstance.GetUpstream(gu.Name)
			Expect(err).NotTo(HaveOccurred())
			return us.Functions
		}, 60, 1).Should(HaveLen(1))

		us, err := glooInstance.GetUpstream(gu.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(us.Functions[0].Name).Should(Equal(funcname))

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

	It("Should work with exsiting function", func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()

		gu := NewGoogleUpstream()

		err = glooInstance.AddUpstream(gu)
		Expect(err).NotTo(HaveOccurred())

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
							UpstreamName: gu.Name,
							FunctionName: gu.Functions[0].Name,
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

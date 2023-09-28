package istio_sds_test

import (
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	kubernetesplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	httpbinName = "httpbin"
	httpbinPort = 8000
)

var _ = Describe("Gloo + Istio SDS integration tests", func() {
	var (
		upstreamRef, virtualServiceRef core.ResourceRef
	)

	BeforeEach(func() {
		virtualServiceRef = core.ResourceRef{Name: httpbinName, Namespace: namespace}

		// the upstream should be created by discovery service
		upstreamRef = core.ResourceRef{
			Name:      kubernetesplugin.UpstreamName(namespace, httpbinName, httpbinPort),
			Namespace: namespace,
		}
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
		})

		route := helpers.NewRouteBuilder().
			WithRouteActionToUpstreamRef(&upstreamRef).
			WithMatcher(&matchers.Matcher{
				PathSpecifier: &matchers.Matcher_Prefix{
					Prefix: "/",
				},
			}).
			Build()

		vs := helpers.NewVirtualServiceBuilder().
			WithName(virtualServiceRef.Name).
			WithNamespace(virtualServiceRef.Namespace).
			WithDomain(httpbinName).
			WithRoute("default-route", route).
			Build()

		_, err := resourceClientSet.VirtualServiceClient().Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
		})
	})

	AfterEach(func() {
		var err error
		err = resourceClientSet.VirtualServiceClient().Delete(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
		helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
			return resourceClientSet.VirtualServiceClient().Read(virtualServiceRef.Namespace, virtualServiceRef.Name, clients.ReadOpts{})
		})

		err = resourceClientSet.UpstreamClient().Delete(upstreamRef.Namespace, upstreamRef.Name, clients.DeleteOpts{
			IgnoreNotExist: true,
		})
		helpers.EventuallyResourceDeleted(func() (resources.InputResource, error) {
			return resourceClientSet.UpstreamClient().Read(upstreamRef.Namespace, upstreamRef.Name, clients.ReadOpts{})
		})
	})

	Context("strict peer auth", func() {
		BeforeEach(func() {
			err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", filepath.Join(cwd, "artifacts", "peerauth_strict.yaml"))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", namespace, "peerauthentication", "test")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should make a request with the expected cert header", func() {
			// the /headers endpoint will respond with the headers the request to the client contains
			testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
				Protocol:          "http",
				Path:              "/headers",
				Method:            "GET",
				Host:              httpbinName,
				Service:           gatewayProxy,
				Port:              gatewayPort,
				ConnectionTimeout: 10,
				Verbose:           false,
				WithoutStats:      true,
				ReturnHeaders:     false,
			}, fmt.Sprintf("\"X-Forwarded-Client-Cert\""), 1, time.Minute*1)
		})
	})
})

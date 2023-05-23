package glooctl_test

import (
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	petstoreYaml     = filepath.Join(util.GetModuleRoot(), "example", "petstore", "petstore.yaml")
	petstoreCurlOpts = helper.CurlOpts{
		Protocol:          "http",
		Path:              "/api/pets",
		Method:            "GET",
		Host:              defaults.GatewayProxyName,
		Service:           defaults.GatewayProxyName,
		Verbose:           true,
		Port:              80,
		ConnectionTimeout: 1,
		WithoutStats:      true,
		Headers: map[string]string{
			"Cache-Control": "no-cache",
		},
	}
	petstoreSuccessfulResponse = `[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]`
)

var _ = Describe("Istio", Ordered, func() {

	// Tests for: `glooctl istio [..]`
	// These tests assume that Gloo and Istio are pre-instaled in the cluster

	BeforeAll(func() {
		err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", petstoreYaml)
		Expect(err).NotTo(HaveOccurred(), "should be able to install petstore")

		_, err = GlooctlOut("add", "route", "--name", "petstore", "--namespace", testHelper.InstallNamespace, "--path-prefix", "/", "--dest-name", "default-petstore-8080", "--dest-namespace", testHelper.InstallNamespace)
		Expect(err).NotTo(HaveOccurred(), "should be able to add gloo route to petstore")

		err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "label", "namespace", "default", "istio-injection=enabled", "--overwrite")
		Expect(err).NotTo(HaveOccurred(), "should be able to add a label to enable istio injection")
	})

	AfterAll(func() {
		err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "label", "namespace", "default", "istio-injection-")
		Expect(err).NotTo(HaveOccurred(), "should be able to remove the istio injection label")

		err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-f", petstoreYaml)
		Expect(err).NotTo(HaveOccurred(), "should be able to uninstall petstore")

		err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "vs", "petstore", "-n", testHelper.InstallNamespace)
		Expect(err).NotTo(HaveOccurred(), "should be able to delete the petstore VS")

		Eventually(func(g Gomega) {
			virtualServices, err := resourceClientset.VirtualServiceClient().List(testHelper.InstallNamespace, clients.ListOpts{
				Ctx: ctx,
			})
			g.Expect(err).NotTo(HaveOccurred(), "should be able to list virtual services")
			g.Expect(virtualServices).To(HaveLen(0), "should have no virtual services")
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	EventuallyIstioInjected := func() {
		trueOffset := 1
		EventuallyWithOffset(trueOffset, func(g Gomega) {
			// Check for sds sidecar
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			g.Expect(sdsContainer).To(Equal("'sds'"), "sds container should be present after injection")
			g.Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			// Check for istio-proxy sidecar
			istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "istio-proxy")].name}'`)
			g.Expect(istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present after injection")
			g.Expect(err).NotTo(HaveOccurred())

			// Check for configMap changes
			configMapEnvoyYAML, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "configmaps", "gateway-proxy-envoy-config", "-o", `jsonpath='{.data}'`)
			g.Expect(configMapEnvoyYAML).To(ContainSubstring("clusterName: gateway_proxy_sds"))
			g.Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")
		}, time.Second*10, time.Second).ShouldNot(HaveOccurred(), "eventually istio injected")
	}

	EventuallyIstioUninjected := func() {
		trueOffset := 1
		EventuallyWithOffset(trueOffset, func(g Gomega) {
			// Check for sds sidecar
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			g.Expect(sdsContainer).To(Equal("''"), "sds container should be removed after uninjection")
			g.Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			// Check for istio-proxy sidecar
			istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "istio-proxy")].name}'`)
			g.Expect(istioContainer).To(Equal("''"), "istio-proxy container should be removed after uninjection")
			g.Expect(err).NotTo(HaveOccurred())

			// Check for configMap changes
			configMapEnvoyYAML, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "configmaps", "gateway-proxy-envoy-config", "-o", `jsonpath='{.data}'`)
			g.Expect(configMapEnvoyYAML).NotTo(ContainSubstring("clusterName: gateway_proxy_sds"), "gateway_proxy_sds cluster should be removed after uninject")
			g.Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")
		}, time.Second*10, time.Second).ShouldNot(HaveOccurred(), "eventually istio uninjected")

	}

	Context("inject", func() {

		It("works on gateway-pod", func() {
			testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, petstoreSuccessfulResponse, 0, 60*time.Second, 1*time.Second)

			_, err := GlooctlOut("istio", "inject", "--namespace", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

			EventuallyIstioInjected()

			_, err = GlooctlOut("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

			err = toggleStictModePetstore(true)
			Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")

			testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, petstoreSuccessfulResponse, 0, 60*time.Second, 1*time.Second)
		})

		AfterEach(func() {
			_, err := GlooctlOut("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")
			Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

			EventuallyIstioUninjected()
		})

	})

	Context("uninject (success)", func() {

		BeforeEach(func() {
			testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, petstoreSuccessfulResponse, 1, 10*time.Second, 1*time.Second)

			_, err := GlooctlOut("istio", "inject", "--namespace", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

			EventuallyIstioInjected()

			err = toggleStictModePetstore(false)
			Expect(err).NotTo(HaveOccurred(), "should be able to disable mtls strict mode on the petstore app")
		})

		When("no upstreams contain sds configuration", func() {

			It("succeeds", func() {
				_, err := GlooctlOut("istio", "uninject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

				EventuallyIstioUninjected()

				// Expect it to work
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, petstoreSuccessfulResponse, 1, 60*time.Second, 1*time.Second)
			})

		})

		When("upstreams contain sds configuration and --include-upstreams=true", func() {

			It("succeeds", func() {
				_, err := GlooctlOut("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

				_, err = GlooctlOut("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

				EventuallyIstioUninjected()

				// Expect it to work
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, petstoreSuccessfulResponse, 1, 60*time.Second, 1*time.Second)
			})
		})

	})

	Context("uninject (failure)", func() {

		BeforeEach(func() {
			testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, petstoreSuccessfulResponse, 1, 10*time.Second, 1*time.Second)

			_, err := runGlooctlCommand("istio", "inject", "--namespace", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

			EventuallyIstioInjected()

			err = toggleStictModePetstore(false)
			Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")
		})

		When("upstreams contain sds configuration and --include-upstreams=false", func() {

			It("fails", func() {
				_, err := GlooctlOut("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

				_, err = GlooctlOut("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "false")
				Expect(err).To(HaveOccurred(), "should not be able to run 'glooctl istio uninject' without errors")
			})

		})

	})

})

func toggleStictModePetstore(strictModeEnabled bool) error {
	yamlPath := testHelper.RootDir + "/test/kube2e/glooctl/petstore_peerauth_permissive.yaml"
	if strictModeEnabled {
		yamlPath = testHelper.RootDir + "/test/kube2e/glooctl/petstore_peerauth_strict.yaml"
	}
	return exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", yamlPath)

}

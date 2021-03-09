package glooctl_test

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kube2e: glooctl", func() {

	const (
		gatewayProxy = defaults.GatewayProxyName
		gatewayPort  = int(80)

		goodResponse = `[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]`
	)

	Context("environment with Istio and Gloo pre-installed", func() {

		var (
			err              error
			petstoreCurlOpts helper.CurlOpts
		)

		BeforeEach(func() {
			// Install Petstore
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", "https://raw.githubusercontent.com/solo-io/gloo/v1.4.12/example/petstore/petstore.yaml")
			Expect(err).NotTo(HaveOccurred(), "should be able to install petstore")

			// Add the gloo route
			err = runGlooctlCommand(strings.Split("add route --name petstore --namespace "+testHelper.InstallNamespace+" --path-prefix / --dest-name default-petstore-8080  --dest-namespace "+testHelper.InstallNamespace, " ")...)
			Expect(err).NotTo(HaveOccurred(), "should be able to add gloo route to petstore")

			// Enable Istio Injection on default namespace
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "label", "namespace", "default", "istio-injection=enabled")
			Expect(err).NotTo(HaveOccurred(), "should be able to add a label to enable istio injection")

			petstoreCurlOpts = helper.CurlOpts{
				Protocol:          "http",
				Path:              "/api/pets",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Verbose:           true,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
				Headers: map[string]string{
					"Cache-Control": "no-cache",
				},
			}
		})

		AfterEach(func() {
			// Disable Istio Injection on default namespace
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "label", "namespace", "default", "istio-injection-")
			Expect(err).NotTo(HaveOccurred(), "should be able to remove the istio injection label")
		})

		ExpectIstioInjected := func() {
			// Check for sds sidecar
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			ExpectWithOffset(1, sdsContainer).To(Equal("'sds'"), "sds container should be present after injection")
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			// Check for istio-proxy sidecar
			istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "istio-proxy")].name}'`)
			ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present after injection")
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			// Check for configMap changes
			configMapEnvoyYAML, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "configmaps", "gateway-proxy-envoy-config", "-o", `jsonpath='{.data}'`)
			ExpectWithOffset(1, configMapEnvoyYAML).To(ContainSubstring("clusterName: gateway_proxy_sds"))
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")
		}

		ExpectIstioUninjected := func() {
			// Check for sds sidecar
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			ExpectWithOffset(1, sdsContainer).To(Equal("''"), "sds container should be removed after uninjection")
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			// Check for istio-proxy sidecar
			istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "istio-proxy")].name}'`)
			ExpectWithOffset(1, istioContainer).To(Equal("''"), "istio-proxy container should be removed after uninjection")
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			// Check for configMap changes
			configMapEnvoyYAML, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "configmaps", "gateway-proxy-envoy-config", "-o", `jsonpath='{.data}'`)
			ExpectWithOffset(1, configMapEnvoyYAML).NotTo(ContainSubstring("clusterName: gateway_proxy_sds"), "gateway_proxy_sds cluster should be removed after uninject")
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")
		}

		Context("istio inject", func() {

			It("works on gateway-pod", func() {
				err = runGlooctlCommand("istio", "inject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

				ExpectIstioInjected()

				// Enable sslConfig on the upstream
				err = runGlooctlCommand("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

				// Enable mTLS mode for the petstore app
				err = toggleStictModePetstore(true)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")

				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})

			AfterEach(func() {
				err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

				ExpectIstioUninjected()
			})

		})

		Context("istio uninject", func() {

			BeforeEach(func() {
				err = runGlooctlCommand("istio", "inject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

				ExpectIstioInjected()

				err = runGlooctlCommand("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

				err = toggleStictModePetstore(true)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")

				// mTLS strict mode enabled
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})

			It("succeeds when no upstreams contain sds configuration", func() {
				// Swap mTLS mode to permissive for the petstore app
				err = toggleStictModePetstore(false)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls permissive mode on the petstore app")

				// Disable sslConfig on the upstream, by deleting the upstream, and allowing UDS to re-create it without the sslConfig
				err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", testHelper.InstallNamespace, "upstream", "default-petstore-8080")
				Expect(err).NotTo(HaveOccurred(), "should be able to delete the petstore upstream")

				err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

				ExpectIstioUninjected()

				// Expect it to work
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})

			It("fails when upstreams contain sds configuration and --include-upstreams=false", func() {
				err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace)
				Expect(err).To(HaveOccurred(), "should not be able to run 'glooctl istio uninject' without errors")
			})

			It("succeeds when upstreams contain sds configuration and --include-upstreams=true", func() {
				// Swap mTLS mode to permissive for the petstore app
				err = toggleStictModePetstore(false)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls permissive mode on the petstore app")

				err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")
				Expect(err).NotTo(HaveOccurred(), "should not be able to run 'glooctl istio uninject' without errors")

				ExpectIstioUninjected()

				// Expect it to work
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})

			AfterEach(func() {
				// Tests may have already successfully run uninject, so we can ignore the error
				_ = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")

				ExpectIstioUninjected()
			})

		})

	})

})

func runGlooctlCommand(args ...string) error {
	glooctlCommand := []string{filepath.Join(testHelper.BuildAssetDir, testHelper.GlooctlExecName)}
	glooctlCommand = append(glooctlCommand, args...)
	return exec.RunCommand(testHelper.RootDir, false, glooctlCommand...)
}

func toggleStictModePetstore(strictModeEnabled bool) error {
	yamlPath := testHelper.RootDir + "/test/kube2e/glooctl/petstore_peerauth_permissive.yaml"
	if strictModeEnabled {
		yamlPath = testHelper.RootDir + "/test/kube2e/glooctl/petstore_peerauth_strict.yaml"
	}
	return exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", yamlPath)

}

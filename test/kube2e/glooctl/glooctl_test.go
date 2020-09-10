package glooctl_test

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/go-utils/testutils/helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kube2e: glooctl", func() {

	const (
		gatewayProxy = defaults.GatewayProxyName
		gatewayPort  = int(80)
	)

	Context("environment with Istio and Gloo pre-installed", func() {

		// Run through the example from the docs
		It("can inject istio into a gateway-pod", func() {
			var err error
			// Enable Istio Injection on default namespace
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "label", "namespace", "default", "istio-injection=enabled")
			Expect(err).NotTo(HaveOccurred(), "should be able to add a label to enable istio injection")

			// Inject Istio
			err = runGlooctlCommand("istio", "inject", "--namespace", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl inject istio' without errors")

			// Check for sidecars
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			Expect(sdsContainer).To(Equal("'sds'"), "sds container should be present after injection")
			Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "istio-proxy")].name}'`)
			Expect(istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present after injection")
			Expect(err).NotTo(HaveOccurred())

			// Check for configMap changes
			configMapEnvoyYAML, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "configmaps", "gateway-proxy-envoy-config", "-o", `jsonpath='{.data}'`)
			Expect(configMapEnvoyYAML).To(ContainSubstring("clusterName: gateway_proxy_sds"))
			Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			// Install Petstore
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", "https://raw.githubusercontent.com/solo-io/gloo/v1.4.12/example/petstore/petstore.yaml")
			Expect(err).NotTo(HaveOccurred(), "should be able to install petstore")

			// Add the gloo route
			err = runGlooctlCommand(strings.Split("add route --name petstore --namespace "+testHelper.InstallNamespace+" --path-prefix / --dest-name default-petstore-8080  --dest-namespace "+testHelper.InstallNamespace, " ")...)
			Expect(err).NotTo(HaveOccurred(), "should be able to add gloo route to petstore")

			// Enable sslConfig on the upstream
			err = runGlooctlCommand(strings.Split("istio enable-mtls --upstream default-petstore-8080 -n "+testHelper.InstallNamespace, " ")...)
			Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

			// Enable mTLS mode for the petstore app
			err = toggleStictModePetstore(true)
			Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")

			// Check that the endpoint works
			co := helper.CurlOpts{
				Protocol:          "http",
				Path:              "/api/pets",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Verbose:           true,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}
			expectedPetsResponse := `[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]`
			testHelper.CurlEventuallyShouldOutput(co, expectedPetsResponse, 1, 60*time.Second, 1*time.Second)
		})

		It("can uninject istio from a gateway-pod", func() {

			// Uninject Istio
			err := runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl uninject istio' without errors")

			// Check for sidecars
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			Expect(sdsContainer).To(Equal("''"), "sds container should be removed after uninjection")
			Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "istio-proxy")].name}'`)
			Expect(istioContainer).To(Equal("''"), "istio-proxy container should be removed after uninjection")
			Expect(err).NotTo(HaveOccurred())

			// Check for configMap changes
			configMapEnvoyYAML, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "configmaps", "gateway-proxy-envoy-config", "-o", `jsonpath='{.data}'`)
			Expect(configMapEnvoyYAML).NotTo(ContainSubstring("clusterName: gateway_proxy_sds"), "gateway_proxy_sds cluster should be removed after uninject")
			Expect(err).NotTo(HaveOccurred(), "should be able to kubectl get the gateway-proxy containers")

			// Check that the endpoint works
			co := helper.CurlOpts{
				Protocol:          "http",
				Path:              "/api/pets",
				Method:            "GET",
				Host:              gatewayProxy,
				Service:           gatewayProxy,
				Verbose:           true,
				Port:              gatewayPort,
				ConnectionTimeout: 1,
				WithoutStats:      true,
			}

			// Shouldn't work, as strict mTLS is still enabled
			waitingResponse := `waiting for reply`
			testHelper.CurlEventuallyShouldOutput(co, waitingResponse, 1, 60*time.Second, 1*time.Second)

			// Swap mTLS mode to permissive for the petstore app
			err = toggleStictModePetstore(false)
			Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls permissive mode on the petstore app")

			// Remove SDS config from the petstore upstream.
			// We do this by simply deleting the upstream. Then UDS will automatically re-create it, without the sslConfig changes we made.
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", testHelper.InstallNamespace, "upstream", "default-petstore-8080")
			Expect(err).NotTo(HaveOccurred(), "should be able to delete the petstore upstream")

			// Expect it to work again now
			goodResponse := `[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]`
			testHelper.CurlEventuallyShouldOutput(co, goodResponse, 1, 60*time.Second, 1*time.Second)
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

package glooctl_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	. "github.com/onsi/ginkgo/v2"
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
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", "https://raw.githubusercontent.com/solo-io/gloo/v1.11.x/example/petstore/petstore.yaml")
			Expect(err).NotTo(HaveOccurred(), "should be able to install petstore")

			// Add the gloo route to petstore
			_, err = runGlooctlCommand("add", "route", "--name", "petstore", "--namespace", testHelper.InstallNamespace, "--path-prefix", "/", "--dest-name", "default-petstore-8080", "--dest-namespace", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to add gloo route to petstore")

			// Enable Istio Injection on default namespace
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "label", "namespace", "default", "istio-injection=enabled", "--overwrite")
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

			// Remove Petstore vs
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "vs", "petstore", "-n", testHelper.InstallNamespace)
			Expect(err).NotTo(HaveOccurred(), "should be able to delete the petstore VS")

			Eventually(func(g Gomega) {
				virtualservices, err := resourceClientset.VirtualServiceClient().List(testHelper.InstallNamespace, clients.ListOpts{})
				g.Expect(err).NotTo(HaveOccurred(), "should be able to list virtual services")
				g.Expect(virtualservices).To(HaveLen(0), "should have no virtual services")
			}, 5*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
		})

		ExpectIstioInjected := func() {
			// Check for sds sidecar
			sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "deployments", "gateway-proxy", "-o", `jsonpath='{.spec.template.spec.containers[?(@.name == "sds")].name}'`)
			ExpectWithOffset(1, sdsContainer).To(Equal("'sds'"), "sds container should be present after injection")
			// check that container is started properly
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
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)

				_, err = runGlooctlCommand("istio", "inject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

				ExpectIstioInjected()

				// Enable sslConfig on the upstream
				_, err = runGlooctlCommand("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

				// Enable mTLS mode for the petstore app
				err = toggleStictModePetstore(true)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")

				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})

			AfterEach(func() {
				_, err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

				ExpectIstioUninjected()
			})

		})

		Context("istio uninject", func() {

			BeforeEach(func() {
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 10*time.Second, 1*time.Second)

				_, err = runGlooctlCommand("istio", "inject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio inject' without errors")

				ExpectIstioInjected()

				_, err = runGlooctlCommand("istio", "enable-mtls", "--upstream", "default-petstore-8080", "-n", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls on the petstore upstream via sslConfig")

				err = toggleStictModePetstore(true)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls strict mode on the petstore app")

				// mTLS strict mode enabled
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 10*time.Second, 1*time.Second)
			})

			AfterEach(func() {
				// Tests may have already successfully run uninject, so we can ignore the error
				_, _ = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")

				ExpectIstioUninjected()
			})

			It("succeeds when no upstreams contain sds configuration", func() {
				// Swap mTLS mode to permissive for the petstore app
				err = toggleStictModePetstore(false)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls permissive mode on the petstore app")

				// Disable sslConfig on the upstream, by deleting the upstream, and allowing UDS to re-create it without the sslConfig
				err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", testHelper.InstallNamespace, "upstream", "default-petstore-8080")
				Expect(err).NotTo(HaveOccurred(), "should be able to delete the petstore upstream")

				_, err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace)
				Expect(err).NotTo(HaveOccurred(), "should be able to run 'glooctl istio uninject' without errors")

				ExpectIstioUninjected()

				// Expect it to work
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})

			It("fails when upstreams contain sds configuration and --include-upstreams=false", func() {
				_, err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace)
				Expect(err).To(HaveOccurred(), "should not be able to run 'glooctl istio uninject' without errors")
			})

			It("succeeds when upstreams contain sds configuration and --include-upstreams=true", func() {
				// Swap mTLS mode to permissive for the petstore app
				err = toggleStictModePetstore(false)
				Expect(err).NotTo(HaveOccurred(), "should be able to enable mtls permissive mode on the petstore app")

				_, err = runGlooctlCommand("istio", "uninject", "--namespace", testHelper.InstallNamespace, "--include-upstreams", "true")
				Expect(err).NotTo(HaveOccurred(), "should not be able to run 'glooctl istio uninject' without errors")

				ExpectIstioUninjected()

				// Expect it to work
				testHelper.CurlEventuallyShouldRespond(petstoreCurlOpts, goodResponse, 1, 60*time.Second, 1*time.Second)
			})
		})
	})

	Context("check", func() {

		BeforeEach(func() {
			// Check that everything is OK
			kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
		})

		It("all checks pass with OK status", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude deployments", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,deployments")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).NotTo(ContainSubstring("Checking deployments..."))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... Skipping proxies because deployments were excluded"))
		})

		It("can exclude pods", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,pods")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).NotTo(ContainSubstring("Checking pods..."))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude upstreams", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,upstreams,virtual-services")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).NotTo(ContainSubstring("Checking upstreams..."))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude upstreamgroups", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,upstreamgroup")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).NotTo(ContainSubstring("Checking upstream groups..."))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude auth-configs", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,auth-configs")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).NotTo(ContainSubstring("Checking auth configs..."))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude rate-limit-configs", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,rate-limit-configs")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).NotTo(ContainSubstring("Checking rate limit configs..."))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude secrets", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,secrets")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).NotTo(ContainSubstring("Checking secrets..."))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude virtual-services", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,virtual-services")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).NotTo(ContainSubstring("Checking virtual services..."))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude gateways", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,gateways")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).NotTo(ContainSubstring("Checking gateways..."))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("can exclude proxies", func() {
			output, err := runGlooctlCommand("check", "-x", "xds-metrics,proxies")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).NotTo(ContainSubstring("Checking proxies..."))
		})

		It("can run a read only check", func() {
			output, err := runGlooctlCommand("check", "--read-only")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
			Expect(output).To(ContainSubstring("Warning: checking proxies with port forwarding is disabled"))
			Expect(output).To(ContainSubstring("Warning: checking xds with port forwarding is disabled"))
		})

		It("can set timeouts (too short)", func() {
			values, err := os.CreateTemp("", "*.yaml")
			Expect(err).NotTo(HaveOccurred())
			_, err = values.Write([]byte(`checkTimeoutSeconds: 1ns`))
			Expect(err).NotTo(HaveOccurred())

			_, err = runGlooctlCommand("check", "-c", values.Name())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
		})

		It("can set timeouts (appropriately)", func() {
			values, err := os.CreateTemp("", "*.yaml")
			Expect(err).NotTo(HaveOccurred())
			_, err = values.Write([]byte(`checkTimeoutSeconds: 300s`))
			Expect(err).NotTo(HaveOccurred())

			_, err = runGlooctlCommand("check", "-c", values.Name())
			Expect(err).ToNot(HaveOccurred())
		})

		It("fails if no gateway proxy deployments", func() {
			err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "scale", "--replicas=0", "deployment", "gateway-proxy", "-n", "gloo-system")
			Expect(err).ToNot(HaveOccurred())
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "scale", "--replicas=0", "deployment", "public-gw", "-n", "gloo-system")
			Expect(err).ToNot(HaveOccurred())

			_, err = runGlooctlCommand("check", "-x", "xds-metrics")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Gloo installation is incomplete: no active gateway-proxy pods exist in cluster"))

			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "scale", "--replicas=1", "deployment", "gateway-proxy", "-n", "gloo-system")
			Expect(err).ToNot(HaveOccurred())
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "scale", "--replicas=1", "deployment", "public-gw", "-n", "gloo-system")
			Expect(err).ToNot(HaveOccurred())
		})

		It("warns if a given gateway proxy deployment has zero replicas", func() {
			err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "scale", "--replicas=0", "deployment", "gateway-proxy", "-n", "gloo-system")
			Expect(err).ToNot(HaveOccurred())

			output, err := runGlooctlCommand("check", "-x", "xds-metrics")
			Expect(err).ToNot(HaveOccurred())
			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking VirtualHostOptions... OK"))
			Expect(output).To(ContainSubstring("Checking RouteOptions... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
			Expect(output).To(ContainSubstring("Warning: gloo-system:gateway-proxy has zero replicas"))
			Expect(output).To(ContainSubstring("No problems detected."))

			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "scale", "--replicas=1", "deployment", "gateway-proxy", "-n", "gloo-system")
			Expect(err).ToNot(HaveOccurred())
		})

		It("reports multiple errors at one time", func() {
			err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", testHelper.RootDir+"/test/kube2e/glooctl/reject-me.yaml")
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", testHelper.RootDir+"/test/kube2e/glooctl/reject-me-too.yaml")
			Expect(err).NotTo(HaveOccurred())

			_, err = runGlooctlCommand("check")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("* Found rejected virtual service by 'gloo-system': default reject-me-too (Reason: 2 errors occurred:"))
			Expect(err.Error()).To(ContainSubstring("* domain conflict: other virtual services that belong to the same Gateway as this one don't specify a domain (and thus default to '*'): [gloo-system.reject-me]"))
			Expect(err.Error()).To(ContainSubstring("* VirtualHost Error: DomainsNotUniqueError. Reason: domain * is shared by the following virtual hosts: [default.reject-me-too gloo-system.reject-me]"))

			Expect(err.Error()).To(ContainSubstring("* Found rejected virtual service by 'gloo-system': gloo-system reject-me (Reason: 2 errors occurred:"))
			Expect(err.Error()).To(ContainSubstring("* domain conflict: other virtual services that belong to the same Gateway as this one don't specify a domain (and thus default to '*'): [default.reject-me-too]"))
			Expect(err.Error()).To(ContainSubstring("* VirtualHost Error: DomainsNotUniqueError. Reason: domain * is shared by the following virtual hosts: [default.reject-me-too gloo-system.reject-me]"))

			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", "gloo-system", "virtualservice", "reject-me")
			Expect(err).NotTo(HaveOccurred())
			err = exec.RunCommand(testHelper.RootDir, false, "kubectl", "delete", "-n", "default", "virtualservice", "reject-me-too")
			Expect(err).NotTo(HaveOccurred())
		})

		It("connection fails on incorrect namespace check", func() {
			_, err := runGlooctlCommand("check", "check", "-n", "not-gloo-system")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Could not communicate with kubernetes cluster: namespaces \"not-gloo-system\" not found"))

			_, err = runGlooctlCommand("check", "-n", "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Warning: The provided label selector (gloo) applies to no pods"))

			output, err := runGlooctlCommand("check", "-p", "not-gloo")
			Expect(err).ToNot(HaveOccurred())
			Expect(output).To(ContainSubstring("Warning: The provided label selector (not-gloo) applies to no pods"))
			Expect(output).To(ContainSubstring("No problems detected."))

			_, err = runGlooctlCommand("check", "-r", "not-gloo-system")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("No namespaces specified are currently being watched (defaulting to 'gloo-system' namespace)"))
		})

		It("fails when scanning with invalid kubecontext", func() {
			_, err := runGlooctlCommand("check", "check", "--kube-context", "not-gloo-context")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Could not get kubernetes client: Error retrieving Kubernetes configuration: context \"not-gloo-context\" does not exist"))
		})

		It("succeeds with correct kubecontext", func() {
			_, err := runGlooctlCommand("check", "check", "--kube-context", "kind-kind")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("check-crds", func() {
		It("validates correct CRDs", func() {
			if testHelper.ReleasedVersion != "" {
				_, err := runGlooctlCommand("check-crds", "--version", testHelper.ChartVersion())
				Expect(err).ToNot(HaveOccurred())
			} else {
				chartUri := filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
				_, err := runGlooctlCommand("check-crds", "--local-chart", chartUri)
				Expect(err).ToNot(HaveOccurred())
			}
		})
		It("fails with CRD mismatch", func() {
			_, err := runGlooctlCommand("check-crds", "--version", "1.9.0")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("One or more CRDs are out of date"))
		})
	})

})

// runGlooctlCommand take a set of arguments for glooctl and then executes local glooctl with these arguments
func runGlooctlCommand(args ...string) (string, error) {
	return GlooctlOut(args...)
}

func toggleStictModePetstore(strictModeEnabled bool) error {
	yamlPath := testHelper.RootDir + "/test/kube2e/glooctl/petstore_peerauth_permissive.yaml"
	if strictModeEnabled {
		yamlPath = testHelper.RootDir + "/test/kube2e/glooctl/petstore_peerauth_strict.yaml"
	}
	return exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", yamlPath)

}

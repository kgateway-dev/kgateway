package glooctl_test

import (
	"os"
	"path/filepath"

	"github.com/solo-io/gloo/test/kube2e"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/testutils/exec"
)

var _ = Describe("Kube2e: glooctl", func() {

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

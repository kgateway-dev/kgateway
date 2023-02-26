package test

import (
	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
)

var _ = Describe("SVC Accnt Test", func() {
	var allTests = func(testCase renderTestCase) {
		Describe(testCase.rendererName, func() {
			var (
				testManifest    TestManifest
				resourceBuilder ResourceBuilder
			)

			prepareMakefile := func(name string, helmFlags []string) {
				resourceBuilder.Name = name
				resourceBuilder.Labels["gloo"] = name

				tm, err := testCase.renderer.RenderManifest(namespace, helmValues{
					valuesArgs: helmFlags,
				})
				Expect(err).NotTo(HaveOccurred(), "Should be able to render the manifest in the service account unit test")
				testManifest = tm
			}

			BeforeEach(func() {
				resourceBuilder = ResourceBuilder{
					Namespace: namespace,
					Labels: map[string]string{
						"app": "gloo",
					},
				}
			})

			It("gloo", func() {
				prepareMakefile("gloo", []string{"global.glooRbac.namespaced=false"})
				testManifest.ExpectServiceAccount(resourceBuilder.GetServiceAccount())
			})

			It("discovery", func() {
				prepareMakefile("discovery", []string{"global.glooRbac.namespaced=false"})
				testManifest.ExpectServiceAccount(resourceBuilder.GetServiceAccount())
			})

			It("gateway-proxy", func() {
				prepareMakefile("gateway-proxy", []string{"global.glooRbac.namespaced=false"})
				svcAccount := resourceBuilder.GetServiceAccount()
				testManifest.ExpectServiceAccount(svcAccount)
			})

			It("gateway-proxy disables svc account", func() {
				prepareMakefile("gateway-proxy", []string{"global.glooRbac.namespaced=false", "gateway.proxyServiceAccount.disableAutomount=true"})
				svcAccount := resourceBuilder.GetServiceAccount()
				falze := false
				svcAccount.AutomountServiceAccountToken = &falze
				testManifest.ExpectServiceAccount(svcAccount)
			})
		})
	}

	runTests(allTests)
})

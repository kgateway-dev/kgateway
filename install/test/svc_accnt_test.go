package test

import (
	. "github.com/onsi/ginkgo"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("SVC Accnt Test", func() {
	var (
		testManifest    TestManifest
		resourceBuilder ResourceBuilder
		installationId  = "svc-accnt-installation-id"
	)

	prepareMakefile := func(name, helmFlags string) {
		resourceBuilder.Name = name
		resourceBuilder.Labels["gloo"] = name
		resourceBuilder.Labels["installationId"] = installationId

		testManifest = renderManifest(helmFlags + " --set global.glooInstallationId=" + installationId)
	}

	BeforeEach(func() {
		resourceBuilder = ResourceBuilder{
			Namespace: namespace,
			Labels: map[string]string{
				"app": "gloo",
			},
			Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "5"},
		}
	})

	It("gloo", func() {
		prepareMakefile("gloo", "--namespace "+namespace+" --set namespace.create=true --set rbac.namespaced=false")
		testManifest.ExpectServiceAccount(resourceBuilder.GetServiceAccount())
	})

	It("discovery", func() {
		prepareMakefile("discovery", "--namespace "+namespace+" --set namespace.create=true --set rbac.namespaced=false")
		testManifest.ExpectServiceAccount(resourceBuilder.GetServiceAccount())
	})

	It("gateway", func() {
		prepareMakefile("gateway", "--namespace "+namespace+" --set namespace.create=true --set rbac.namespaced=false")
		testManifest.ExpectServiceAccount(resourceBuilder.GetServiceAccount())
	})

	It("gateway-proxy", func() {
		prepareMakefile("gateway-proxy", "--namespace "+namespace+" --set namespace.create=true --set rbac.namespaced=false")
		svcAccount := resourceBuilder.GetServiceAccount()
		testManifest.ExpectServiceAccount(svcAccount)
	})

	It("gateway-proxy disables svc account", func() {
		prepareMakefile("gateway-proxy", "--namespace "+namespace+" --set namespace.create=true --set rbac.namespaced=false --set gateway.proxyServiceAccount.disableAutomount=true")
		svcAccount := resourceBuilder.GetServiceAccount()
		falze := false
		svcAccount.AutomountServiceAccountToken = &falze
		testManifest.ExpectServiceAccount(svcAccount)
	})

})

package test

import (
	. "github.com/onsi/ginkgo"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("SVC Accnt Test", func() {
	var (
		testManifest    TestManifest
		resourceBuilder ResourceBuilder
	)

	prepareMakefile := func(name, helmFlags string) {
		resourceBuilder.Name = name
		resourceBuilder.Labels["gloo"] = name

		testManifest = renderManifest(helmFlags)
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
		falze := false
		svcAccount.AutomountServiceAccountToken = &falze
		testManifest.ExpectServiceAccount(svcAccount)
	})

})

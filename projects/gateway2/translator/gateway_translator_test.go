package translator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("GatewayTranslator", func() {
	ctx := context.TODO()
	dir := util.MustGetThisDir()

	It("should translate a gateway with basic routing", func() {
		results, err := TestCase{
			Name:       "basic-http-routing",
			InputFiles: []string{dir + "/testutils/inputs/http-routing"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "example-gateway",
				}: {
					Proxy: dir + "/testutils/outputs/http-routing-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results).To(HaveKey(types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}))
		Expect(results[types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}]).To(BeTrue())
	})

	It("should translate a gateway with https routing", func() {
		results, err := TestCase{
			Name:       "basic-http-routing",
			InputFiles: []string{dir + "/testutils/inputs/https-routing"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "example-gateway",
				}: {
					Proxy: dir + "/testutils/outputs/https-routing-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results).To(HaveKey(types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}))
		Expect(results[types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}]).To(BeTrue())
	})
})

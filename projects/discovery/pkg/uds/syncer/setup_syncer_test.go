package syncer

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("UDS setup syncer tests", func() {

	Context("RunUDS", func() {
		It("returns nil when UDS is disabled", func() {
			opts := bootstrap.Opts{
				Settings: &v1.Settings{
					Metadata: &core.Metadata{
						Name:      "test-settings",
						Namespace: "gloo-system",
					},
					Discovery: &v1.Settings_DiscoveryOptions{
						UdsOptions: &v1.Settings_DiscoveryOptions_UdsOptions{
							Enabled: false,
						},
					},
				},
			}
			Expect(RunUDS(opts)).To(BeNil())
		})
	})

})

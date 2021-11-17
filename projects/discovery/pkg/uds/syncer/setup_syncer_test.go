package syncer

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("UDS setup syncer tests", func() {

	Context("GetUdsEnabled", func() {
		It("returns true when settings is nil", func() {
			Expect(GetUdsEnabled(nil)).To(BeTrue())
		})
		It("returns true when settings.discovery is nil", func() {
			settings := &v1.Settings{
				Discovery: nil,
			}
			Expect(GetUdsEnabled(settings)).To(BeTrue())
		})
		It("returns true when settings.discovery.udsEnabled is true", func() {
			settings := getSettings(true)
			Expect(GetUdsEnabled(settings)).To(BeTrue())
		})
		It("returns false when settings.discovery.udsEnabled is false", func() {
			settings := getSettings(false)
			Expect(GetUdsEnabled(settings)).To(BeFalse())
		})
	})

	Context("RunUDS", func() {
		It("returns nil when UDS is disabled", func() {
			opts := bootstrap.Opts{
				Settings: getSettings(false),
			}
			Expect(RunUDS(opts)).To(BeNil())
		})
	})

})

// Helper for creating settings object with only the necessary fields
func getSettings(udsEnabled bool) *v1.Settings {
	return &v1.Settings{
		// Not necessary for tests to pass, but nice to have to ensure RunUDS() logs correctly
		Metadata: &core.Metadata{
			Name:      "test-settings",
			Namespace: "gloo-system",
		},
		Discovery: &v1.Settings_DiscoveryOptions{
			UdsEnabled: udsEnabled,
		},
	}
}

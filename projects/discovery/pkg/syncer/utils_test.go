package syncer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("Discovery Syncer Utils Tests", func() {

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
		It("returns true when settings.discovery.udsOptions is nil", func() {
			settings := &v1.Settings{
				Discovery: &v1.Settings_DiscoveryOptions{
					UdsOptions: nil,
				},
			}
			Expect(GetUdsEnabled(settings)).To(BeTrue())
		})
		It("returns true when settings.discovery.udsOptions.enabled is nil", func() {
			settings := &v1.Settings{
				Discovery: &v1.Settings_DiscoveryOptions{
					UdsOptions: &v1.Settings_DiscoveryOptions_UdsOptions{
						Enabled: nil,
					},
				},
			}
			Expect(GetUdsEnabled(settings)).To(BeTrue())
		})
		It("returns true when settings.discovery.udsOptions.enabled is true", func() {
			settings := getSettings(true)
			Expect(GetUdsEnabled(settings)).To(BeTrue())
		})
		It("returns false when settings.discovery.udsOptions.enabled is false", func() {
			settings := getSettings(false)
			Expect(GetUdsEnabled(settings)).To(BeFalse())
		})
	})

	Context("GetWatchLabels", func() {
		It("returns nil when settings is nil", func() {
			Expect(GetWatchLabels(nil)).To(BeNil())
		})

		It("returns nil when settings.discovery is nil", func() {
			settings := &v1.Settings{
				Discovery: nil,
			}
			Expect(GetWatchLabels(settings)).To(BeNil())
		})

		It("returns WatchLabels when set", func() {
			watchLabels := map[string]string{"A": "B"}
			settings := &v1.Settings{
				Discovery: &v1.Settings_DiscoveryOptions{
					UdsOptions: &v1.Settings_DiscoveryOptions_UdsOptions{
						WatchLabels: watchLabels,
					},
				},
			}
			Expect(GetWatchLabels(settings)).To(BeEquivalentTo(map[string]string{"A": "B"}))
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
			UdsOptions: &v1.Settings_DiscoveryOptions_UdsOptions{
				Enabled: &wrappers.BoolValue{Value: udsEnabled},
			},
		},
	}
}

package runner_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	udsrunner "github.com/solo-io/gloo/projects/discovery/pkg/uds/runner"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/golang/protobuf/ptypes/wrappers"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("RunUDS", func() {

	It("returns an error when both UDS and FDS are disabled", func() {
		opts := runner.RunOpts{
			Settings: &v1.Settings{
				Metadata: &core.Metadata{
					Name:      "test-settings",
					Namespace: "gloo-system",
				},
				Discovery: &v1.Settings_DiscoveryOptions{
					UdsOptions: &v1.Settings_DiscoveryOptions_UdsOptions{
						Enabled: &wrappers.BoolValue{Value: false},
					},
					FdsMode: v1.Settings_DiscoveryOptions_DISABLED,
				},
			},
		}
		Expect(udsrunner.RunUDS(opts)).To(HaveOccurred())
	})

	It("Does not return an error when WatchLabels are set", func() {
		memcache := memory.NewInMemoryResourceCache()
		settings := &v1.Settings{
			Metadata: &core.Metadata{
				Name:      "test-settings",
				Namespace: "gloo-system",
			},
			Discovery: &v1.Settings_DiscoveryOptions{
				UdsOptions: &v1.Settings_DiscoveryOptions_UdsOptions{
					Enabled:     &wrappers.BoolValue{Value: true},
					WatchLabels: map[string]string{"A": "B"},
				},
				FdsMode: v1.Settings_DiscoveryOptions_DISABLED,
			},
		}

		glooClientset, _, err := runner.GenerateGlooClientsets(context.Background(), settings, nil, memcache)
		Expect(err).NotTo(HaveOccurred())

		opts := runner.RunOpts{
			Settings:          settings,
			ResourceClientset: glooClientset,
		}
		Expect(udsrunner.RunUDS(opts)).NotTo(HaveOccurred())
	})

})

package runner_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/runner"
	gloorunner "github.com/solo-io/gloo/projects/gloo/pkg/runner"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("RunFDS", func() {

	It("returns an error when both UDS and FDS are disabled", func() {
		opts := gloorunner.RunOpts{
			Settings: &gloov1.Settings{
				Metadata: &core.Metadata{
					Name:      "test-settings",
					Namespace: "gloo-system",
				},
				Discovery: &gloov1.Settings_DiscoveryOptions{
					UdsOptions: &gloov1.Settings_DiscoveryOptions_UdsOptions{
						Enabled: &wrappers.BoolValue{Value: false},
					},
					FdsMode: gloov1.Settings_DiscoveryOptions_DISABLED,
				},
			},
		}
		Expect(runner.RunFDS(opts)).To(HaveOccurred())
	})

})

package runner_test

import (
	"context"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/bootstrap"
	fdsrunner "github.com/solo-io/gloo/projects/discovery/pkg/fds/runner"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("FDS Runner", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		runner bootstrap.Runner
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		runner = fdsrunner.NewFDSRunner()
	})

	AfterEach(func() {
		cancel()
	})

	It("returns an error when both UDS and FDS are disabled", func() {
		settings := &gloov1.Settings{
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
		}

		err := runner.Run(ctx, nil, nil, settings)
		Expect(err).To(HaveOccurred())
	})

})

package runner_test

import (
	"context"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/bootstrap"
	udsrunner "github.com/solo-io/gloo/projects/discovery/pkg/uds/runner"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("UDS Runner", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		runner bootstrap.Runner
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		runner = udsrunner.NewUDSRunner()
	})

	AfterEach(func() {
		cancel()
	})

	It("returns an error when both UDS and FDS are disabled", func() {
		settings := &v1.Settings{
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
		}

		err := runner.Run(ctx, nil, nil, settings)
		Expect(err).To(HaveOccurred())
	})

	It("Does not return an error when WatchLabels are set", func() {
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

		err := runner.Run(ctx, nil, memory.NewInMemoryResourceCache(), settings)
		Expect(err).NotTo(HaveOccurred())
	})

})

package bootstrap_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/bootstrap"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("SetupSyncer", func() {

	It("calls the RunFunc with the referenced settings crd", func() {
		var actualSettings *v1.Settings
		expectedSettings := &v1.Settings{
			Metadata: &core.Metadata{Name: "hello", Namespace: "goodbye"},
		}

		mockRunFunc := func() error {
			actualSettings = expectedSettings
			return nil
		}

		mockRunnerFactory := func(
			ctx context.Context,
			kubeCache kube.SharedCache,
			inMemoryCache memory.InMemoryResourceCache,
			settings *v1.Settings,
		) (bootstrap.RunFunc, error) {
			return mockRunFunc, nil
		}

		setupSyncer := bootstrap.NewSetupSyncer(expectedSettings.Metadata.Ref(), mockRunnerFactory)
		err := setupSyncer.Sync(context.TODO(), &v1.SetupSnapshot{
			Settings: v1.SettingsList{expectedSettings},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(actualSettings).To(Equal(expectedSettings))
	})
})

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

	It("Executes the Runner with the referenced settings crd", func() {
		expectedSettings := &v1.Settings{
			Metadata: &core.Metadata{Name: "hello", Namespace: "goodbye"},
		}

		runner := &mockRunner{lastRunSettings: nil}
		setupSyncer := bootstrap.NewSetupSyncer(expectedSettings.Metadata.Ref(), runner)
		err := setupSyncer.Sync(context.TODO(), &v1.SetupSnapshot{
			Settings: v1.SettingsList{expectedSettings},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(runner.lastRunSettings).To(Equal(expectedSettings))
	})
})

var _ bootstrap.Runner = new(mockRunner)

type mockRunner struct {
	lastRunSettings *v1.Settings
}

func (m *mockRunner) Run(_ context.Context, _ kube.SharedCache, _ memory.InMemoryResourceCache, settings *v1.Settings) error {
	m.lastRunSettings = settings
	return nil
}

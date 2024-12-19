package setup

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
)

var _ = Describe("Extensions", func() {

	DescribeTable("Validate returns expected error",
		func(extensions Extensions, expectedError types.GomegaMatcher) {
			Expect(extensions.Validate()).To(expectedError)
		},
		Entry("missing SnapshotHistoryFactory", Extensions{
			SnapshotHistoryFactory: nil,
		}, MatchError(ErrNilExtension("SnapshotHistoryFactory"))),
		Entry("missing PluginRegistryFactory", Extensions{
			SnapshotHistoryFactory: iosnapshot.GetHistoryFactory(),
			PluginRegistryFactory:  nil,
		}, MatchError(ErrNilExtension("PluginRegistryFactory"))),
		Entry("missing ApiEmitterChannel", Extensions{
			SnapshotHistoryFactory: iosnapshot.GetHistoryFactory(),
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: nil,
		}, MatchError(ErrNilExtension("ApiEmitterChannel"))),
		Entry("missing SyncerExtensions", Extensions{
			SnapshotHistoryFactory: iosnapshot.GetHistoryFactory(),
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: make(chan struct{}),
			SyncerExtensions:  nil,
		}, MatchError(ErrNilExtension("SyncerExtensions"))),
		Entry("missing nothing", Extensions{
			SnapshotHistoryFactory: iosnapshot.GetHistoryFactory(),
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: make(chan struct{}),
			SyncerExtensions:  make([]syncer.TranslatorSyncerExtensionFactory, 0),
		}, BeNil()),
	)

})

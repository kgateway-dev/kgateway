//go:build exclude

package setup

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
)

var _ = Describe("Extensions", func() {

	DescribeTable("Validate returns expected error",
		func(extensions Extensions, expectedError types.GomegaMatcher) {
			Expect(extensions.Validate()).To(expectedError)
		},
		Entry("missing PluginRegistryFactory", Extensions{
			PluginRegistryFactory: nil,
		}, MatchError(ErrNilExtension("PluginRegistryFactory"))),
		Entry("missing ApiEmitterChannel", Extensions{
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: nil,
		}, MatchError(ErrNilExtension("ApiEmitterChannel"))),
		Entry("missing SyncerExtensions", Extensions{
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: make(chan struct{}),
			SyncerExtensions:  nil,
		}, MatchError(ErrNilExtension("SyncerExtensions"))),
		Entry("missing nothing", Extensions{
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: make(chan struct{}),
			SyncerExtensions:  make([]syncer.TranslatorSyncerExtensionFactory, 0),
		}, BeNil()),
	)

})

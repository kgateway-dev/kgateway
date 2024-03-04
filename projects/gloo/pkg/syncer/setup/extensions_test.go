package setup

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
)

var _ = Describe("Extensions", func() {

	DescribeTable("Validate returns expected error",
		func(extensions Extensions, expectedError types.GomegaMatcher) {
			Expect(extensions.Validate()).To(expectedError)
		},
		Entry("missing K8sGatewayExtensions", Extensions{
			K8sGatewayExtensions: nil,
		}, MatchError("Extensions.K8sGatewayExtension must be defined, found nil")),
		Entry("missing PluginRegistryFactory", Extensions{
			K8sGatewayExtensions:  extensions.NewManager,
			PluginRegistryFactory: nil,
		}, MatchError("Extensions.PluginRegistryFactory must be defined, found nil")),
		Entry("missing ApiEmitterChannel", Extensions{
			K8sGatewayExtensions: extensions.NewManager,
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: nil,
		}, MatchError("Extensions.ApiEmitterChannel must be defined, found nil")),
		Entry("missing ApiEmitterChannel", Extensions{
			K8sGatewayExtensions: extensions.NewManager,
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: make(chan struct{}),
			SyncerExtensions:  nil,
		}, MatchError("Extensions.SyncerExtensions must be defined, found nil")),
		Entry("missing nothing", Extensions{
			K8sGatewayExtensions: extensions.NewManager,
			PluginRegistryFactory: func(ctx context.Context) plugins.PluginRegistry {
				// non-nil function
				return nil
			},
			ApiEmitterChannel: make(chan struct{}),
			SyncerExtensions:  make([]syncer.TranslatorSyncerExtensionFactory, 0),
		}, BeNil()),
	)

})

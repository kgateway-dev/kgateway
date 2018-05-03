package snapshot_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	secretwatchersetup "github.com/solo-io/gloo/pkg/bootstrap/secretwatcher"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/endpointswatcher"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

var opts bootstrap.Options

var _ = Describe("Emitter", func() {
	Describe("Snapshot()", func() {
		Context("using kubernetes for config, endpoints, secrets, and files", func() {
			It("sends snapshots down the channel", func() {
				store, err := configstorage.Bootstrap(opts)
				Expect(err).NotTo(BeNil())
				cfgWatcher, err := configwatcher.NewConfigWatcher(store)
				Expect(err).NotTo(BeNil())
				secretWatcher, err := secretwatchersetup.Bootstrap(opts)
				filestore, err := artifactstorage.Bootstrap(opts)
				Expect(err).NotTo(BeNil())
				fileWatcher, err := filewatcher.NewFileWatcher(filestore)
				Expect(err).NotTo(BeNil())
				endpointsWatcher := endpointswatcher.NewEndpointsWatcher(opts, &kubernetes.Plugin{})
				getDependencies := func(cfg *v1.Config) []*plugins.Dependencies {
					return nil
				}
				emitter := NewEmitter(cfgWatcher, secretWatcher, fileWatcher, endpointsWatcher, getDependencies)
			})
		})
	})
})

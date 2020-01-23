package syncer_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
)

var _ = Describe("NoWatchArtifactClient", func() {

	When("calling watch function", func() {
		FIt("result in a no-op", func() {
			rcFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}

			client, err := syncer.NewNoWatchArtifactClient(rcFactory)
			Expect(err).NotTo(HaveOccurred())

			artifactChan, errChan, err := client.Watch("", clients.WatchOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(errChan).To(BeNil())
			Expect(artifactChan).To(BeNil())

			select {
			case <-artifactChan:
				Fail("received unexpected event on no-op chan")
			case <-time.After(50 * time.Millisecond):
				Succeed()
			}
		})
	})
})

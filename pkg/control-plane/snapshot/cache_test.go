package snapshot

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

var _ = Describe("Cache", func() {
	Describe("hash function", func() {
		It("computes equal hashes for the same cache", func() {
			c := newCache()
			c.Cfg = helpers.NewTestConfig()
			Expect(c.Hash()).To(Equal(c.Hash()))
		})
		It("computes equal hashes for the same cache", func() {
			c := newCache()
			c.Cfg = helpers.NewTestConfig()
			h1 := c.Hash()
			c.Cfg = helpers.NewTestConfig()
			Expect(h1).To(Equal(c.Hash()))
			c.Secrets = helpers.NewTestSecrets()
			h1 = c.Hash()
			c.Secrets = helpers.NewTestSecrets()
			Expect(h1).To(Equal(c.Hash()))
		})
		It("computes equal hashes for the same cache", func() {
			c := newCache()
			c.Cfg = helpers.NewTestConfig()
			h1 := c.Hash()
			c.Cfg = helpers.NewTestConfig()
			Expect(h1).To(Equal(c.Hash()))
			c.Secrets = helpers.NewTestSecrets()
			h1 = c.Hash()
			c.Secrets = helpers.NewTestSecrets()
			Expect(h1).To(Equal(c.Hash()))
		})
		It("ignores status on config objects", func() {
			c := newCache()
			c.Cfg = helpers.NewTestConfig()
			h1 := c.Hash()
			c.Cfg.Upstreams[0].Status = &v1.Status{
				Reason: "idk",
			}
			c.Cfg.VirtualServices[0].Status = &v1.Status{
				Reason: "idk",
			}
			Expect(h1).To(Equal(c.Hash()))
		})
		It("ignores resource version on config objects", func() {
			c := newCache()
			c.Cfg = helpers.NewTestConfig()
			c.Cfg.Upstreams[0].Metadata = &v1.Metadata{
				ResourceVersion: "1",
			}
			c.Cfg.VirtualServices[0].Metadata = &v1.Metadata{
				ResourceVersion: "1",
			}
			h1 := c.Hash()
			c.Cfg.Upstreams[0].Metadata = &v1.Metadata{
				ResourceVersion: "2",
			}
			c.Cfg.VirtualServices[0].Metadata = &v1.Metadata{
				ResourceVersion: "2",
			}
			Expect(h1).To(Equal(c.Hash()))
		})
	})
})

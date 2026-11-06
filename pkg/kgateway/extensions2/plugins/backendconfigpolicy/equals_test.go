package backendconfigpolicy

import (
	"testing"
	"time"

	envoycommondnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/common/dns/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"
)

// An incomplete Equals silently breaks KRT change detection: the collection keeps
// serving the stale IR because it believes nothing changed. These cases flip one
// field at a time so a field added to the struct but not to Equals is caught.
func TestBackendConfigPolicyIREquals(t *testing.T) {
	base := func() *BackendConfigPolicyIR {
		return &BackendConfigPolicyIR{
			connectTimeout:  durationpb.New(5 * time.Second),
			dnsRefreshRate:  durationpb.New(60 * time.Second),
			dnsJitter:       durationpb.New(15 * time.Second),
			respectDnsTtl:   new(true),
			dnsLookupFamily: new(envoycommondnsv3.DnsLookupFamily_V4_PREFERRED),
		}
	}

	t.Run("identical IRs are equal", func(t *testing.T) {
		assert.True(t, base().Equals(base()))
	})

	t.Run("differing dnsLookupFamily is not equal", func(t *testing.T) {
		a, b := base(), base()
		b.dnsLookupFamily = new(envoycommondnsv3.DnsLookupFamily_ALL)
		assert.False(t, a.Equals(b), "a changed lookup family must invalidate the cached IR")
		assert.False(t, b.Equals(a), "and the comparison must be symmetric")
	})

	t.Run("dnsLookupFamily set on one side only is not equal", func(t *testing.T) {
		a, b := base(), base()
		b.dnsLookupFamily = nil
		assert.False(t, a.Equals(b), "dropping the lookup family must invalidate the cached IR")
		assert.False(t, b.Equals(a), "and the comparison must be symmetric")
	})

	t.Run("both nil dnsLookupFamily is equal", func(t *testing.T) {
		a, b := base(), base()
		a.dnsLookupFamily = nil
		b.dnsLookupFamily = nil
		assert.True(t, a.Equals(b))
	})

	// The neighbouring DNS fields, so a regression in the block this was added to
	// does not pass unnoticed.
	t.Run("differing neighbouring dns fields are not equal", func(t *testing.T) {
		a, b := base(), base()
		b.respectDnsTtl = new(false)
		assert.False(t, a.Equals(b))

		a, b = base(), base()
		b.dnsRefreshRate = durationpb.New(30 * time.Second)
		assert.False(t, a.Equals(b))

		a, b = base(), base()
		b.dnsJitter = durationpb.New(1 * time.Second)
		assert.False(t, a.Equals(b))
	})

	t.Run("a different type is not equal", func(t *testing.T) {
		assert.False(t, base().Equals("not a policy IR"))
	})
}

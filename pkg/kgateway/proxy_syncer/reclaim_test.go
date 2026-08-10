package proxy_syncer

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortenReclaimGrace shrinks the grace period for the duration of a test.
// Tests using it must not run in parallel.
func shortenReclaimGrace(t *testing.T, d time.Duration) {
	t.Helper()
	orig := snapshotReclaimGracePeriod
	snapshotReclaimGracePeriod = d
	t.Cleanup(func() { snapshotReclaimGracePeriod = orig })
}

// fakeClearer records ClearSnapshot calls.
type fakeClearer struct {
	mu      sync.Mutex
	cleared []string
}

func (f *fakeClearer) ClearSnapshot(node string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared = append(f.cleared, node)
}

func (f *fakeClearer) clearedNodes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cleared...)
}

// connectedSet is a thread-safe stand-in for the connected-client membership
// check.
type connectedSet struct {
	mu  sync.Mutex
	set map[string]bool
}

func newConnectedSet() *connectedSet { return &connectedSet{set: map[string]bool{}} }

func (c *connectedSet) setConnected(name string, connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set[name] = connected
}

func (c *connectedSet) stillDeparted(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.set[name]
}

func TestReclaim_DepartedEntryClearedAfterGrace(t *testing.T) {
	shortenReclaimGrace(t, 30*time.Millisecond)
	clearer := &fakeClearer{}
	conn := newConnectedSet()
	r := newSnapshotReclaimer(clearer, conn.stillDeparted)

	r.clientDeparted("c1")
	assert.Empty(t, clearer.clearedNodes(), "must not clear before the grace period")

	require.Eventually(t, func() bool {
		return len(clearer.clearedNodes()) == 1
	}, 2*time.Second, 5*time.Millisecond, "departed client's entry must be cleared after the grace period")
	assert.Equal(t, []string{"c1"}, clearer.clearedNodes())
}

func TestReclaim_ReconnectWithinGraceCancels(t *testing.T) {
	shortenReclaimGrace(t, 40*time.Millisecond)
	clearer := &fakeClearer{}
	conn := newConnectedSet()
	r := newSnapshotReclaimer(clearer, conn.stillDeparted)

	r.clientDeparted("c1")
	conn.setConnected("c1", true)
	r.clientConnected("c1")

	time.Sleep(120 * time.Millisecond)
	assert.Empty(t, clearer.clearedNodes(), "a client that reconnected within the grace period must not be reclaimed")
}

func TestReclaim_FireTimeMembershipRecheck(t *testing.T) {
	// The timer can race a reconnect whose Add event has not reached
	// clientConnected yet; the membership re-check at fire time must win.
	shortenReclaimGrace(t, 30*time.Millisecond)
	clearer := &fakeClearer{}
	conn := newConnectedSet()
	r := newSnapshotReclaimer(clearer, conn.stillDeparted)

	r.clientDeparted("c1")
	conn.setConnected("c1", true) // reconnected, but clientConnected never called

	time.Sleep(120 * time.Millisecond)
	assert.Empty(t, clearer.clearedNodes(), "a client present in the connected set at fire time must not be reclaimed")
}

func TestReclaim_DuplicateDepartsArmOneTimer(t *testing.T) {
	shortenReclaimGrace(t, 30*time.Millisecond)
	clearer := &fakeClearer{}
	conn := newConnectedSet()
	r := newSnapshotReclaimer(clearer, conn.stillDeparted)

	r.clientDeparted("c1")
	r.clientDeparted("c1")
	r.clientDeparted("c1")

	require.Eventually(t, func() bool {
		return len(clearer.clearedNodes()) >= 1
	}, 2*time.Second, 5*time.Millisecond)
	time.Sleep(80 * time.Millisecond)
	assert.Len(t, clearer.clearedNodes(), 1, "duplicate departure events must not produce duplicate reclaims")
}

func TestReclaim_IndependentClients(t *testing.T) {
	shortenReclaimGrace(t, 30*time.Millisecond)
	clearer := &fakeClearer{}
	conn := newConnectedSet()
	r := newSnapshotReclaimer(clearer, conn.stillDeparted)

	r.clientDeparted("gone")
	r.clientDeparted("back")
	conn.setConnected("back", true)
	r.clientConnected("back")

	require.Eventually(t, func() bool {
		return len(clearer.clearedNodes()) == 1
	}, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"gone"}, clearer.clearedNodes(),
		"only the still-departed client is reclaimed")
}

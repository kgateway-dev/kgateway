package proxy_syncer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// testSignalRow is a minimal row: name is its key and version its content.
type testSignalRow struct {
	name    string
	version int
}

func (r testSignalRow) ResourceName() string { return r.name }

func (r testSignalRow) Equals(in testSignalRow) bool {
	return r.name == in.name && r.version == in.version
}

func TestChangeSignalsFollowRows(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	rows := krt.NewMutableCollection(nil, []testSignalRow{{name: "a", version: 1}}, krt.WithStop(stop))
	signals := changeSignals(krtutil.NewKrtOptions(stop, nil), "TestSignals", rows.AsCollection())
	events := make(chan krt.Event[string], 8)
	reg := signals.Register(func(e krt.Event[string]) { events <- e })
	t.Cleanup(reg.UnregisterHandler)
	require.True(t, reg.WaitUntilSynced(stop))
	readEvent := func() krt.Event[string] {
		t.Helper()
		select {
		case e := <-events:
			return e
		case <-time.After(2 * time.Second):
			t.Fatal("missing key event")
			return krt.Event[string]{}
		}
	}
	key := "a"
	assert.Equal(t, krt.Event[string]{Event: controllers.EventAdd, New: &key}, readEvent())

	for version := 2; version <= 4; version++ {
		rows.UpdateObject(testSignalRow{name: "a", version: version})
		assert.Equal(t, krt.Event[string]{Event: controllers.EventUpdate, Old: &key, New: &key}, readEvent(),
			"updates must be forwarded even though the projected keys are equal")
	}

	rows.DeleteObject("a")
	assert.Equal(t, krt.Event[string]{Event: controllers.EventDelete, Old: &key}, readEvent())
	assert.Nil(t, signals.GetKey("a"))
}

func TestFetchSignaledMissingRow(t *testing.T) {
	ctx := krt.TestingDummyContext{}
	row := testSignalRow{name: "a", version: 1}
	rows := krt.NewStaticCollection(nil, []testSignalRow{row})
	signals := changeSignals(krtutil.KrtOptions{}, "TestSignals", rows)
	assert.Nil(t, fetchSignaled(ctx, signals, rows, "missing"))

	got := fetchSignaled(ctx, signals, rows, "a")
	require.NotNil(t, got)
	assert.Equal(t, row, *got)
}

// Reads must not wait for a separate signal collection to process its input
// queue. Delay delivery of a source update while its stored rows move ahead.
func TestChangeSignalsReadCurrentRowsWhileEventsBlocked(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	krtopts := krtutil.NewKrtOptions(stop, nil)
	rows := krt.NewMutableCollection(nil, []testSignalRow{{name: "a", version: 1}}, krt.WithStop(stop))
	blocked := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var stall atomic.Bool
	var once sync.Once
	source := krt.MapCollection(rows.AsCollection(), func(row testSignalRow) testSignalRow {
		if row.version == 1 && stall.Load() {
			once.Do(func() {
				close(blocked)
				<-release
			})
		}
		return row
	}, krtopts.ToOptions("DelayedEvents")...)
	signals := changeSignals(krtopts, "TestSignals", source)
	reg := signals.Register(func(krt.Event[string]) {})
	t.Cleanup(reg.UnregisterHandler)
	require.True(t, reg.WaitUntilSynced(stop))

	stall.Store(true)
	rows.UpdateObject(testSignalRow{name: "a", version: 2})
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("source event delivery did not block")
	}
	rows.DeleteObject("a")
	rows.UpdateObject(testSignalRow{name: "b", version: 3})

	assert.Nil(t, signals.GetKey("a"), "a deleted row must not remain in a separate signal cache")
	require.NotNil(t, signals.GetKey("b"), "a new row must be readable before its event is delivered")
	assert.Equal(t, &testSignalRow{name: "b", version: 3},
		fetchSignaled(krt.TestingDummyContext{}, signals, source, "b"))
}

// A transform that reads through a signal follows the row: it reruns on every
// change and ends on the latest row, as it would fetching the row itself.
func TestFetchSignaledTransformReadsLatestRow(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	krtopts := krtutil.NewKrtOptions(stop, nil)
	rows := krt.NewMutableCollection(nil, []testSignalRow{{name: "a", version: 1}}, krt.WithStop(stop))
	signals := changeSignals(krtopts, "TestSignals", rows.AsCollection())
	keys := krt.NewStaticCollection(nil, []string{"a"})
	blocked := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var once sync.Once
	reads := krt.NewCollection(keys, func(kctx krt.HandlerContext, key string) *testSignalRow {
		row := fetchSignaled(kctx, signals, rows.AsCollection(), key)
		if row != nil && row.version == 2 {
			once.Do(func() {
				close(blocked)
				<-release
			})
		}
		return row
	}, krtopts.ToOptions("TestReads")...)
	reads.WaitUntilSynced(stop)

	readsVersion := func(version int) func() bool {
		return func() bool {
			r := reads.GetKey("a")
			return r != nil && r.version == version
		}
	}
	require.Eventually(t, readsVersion(1), 2*time.Second, 5*time.Millisecond)

	rows.UpdateObject(testSignalRow{name: "a", version: 2})
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("an update with equal projected keys did not rerun the reader")
	}
	for version := 3; version <= 20; version++ {
		rows.UpdateObject(testSignalRow{name: "a", version: version})
	}
	unblock()
	require.Eventually(t, readsVersion(20), 2*time.Second, 5*time.Millisecond, "the reader ends on the latest row")

	rows.DeleteObject("a")
	require.Eventually(t, func() bool {
		return reads.GetKey("a") == nil
	}, 2*time.Second, 5*time.Millisecond, "a deleted row reruns the reader")

	rows.UpdateObject(testSignalRow{name: "a", version: 20})
	require.Eventually(t, readsVersion(20), 2*time.Second, 5*time.Millisecond, "a re-added identical row reruns the reader")
}

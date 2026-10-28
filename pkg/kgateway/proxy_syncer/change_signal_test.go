package proxy_syncer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestChangeSignalEquals(t *testing.T) {
	s := changeSignal{key: "a", n: 1}
	assert.True(t, s.Equals(changeSignal{key: "a", n: 1}))
	assert.False(t, s.Equals(changeSignal{key: "a", n: 2}), "a new count is a change")
	assert.False(t, s.Equals(changeSignal{key: "b", n: 1}), "the key is compared")
}

func TestChangeSignalsFollowRows(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	rows := krt.NewMutableCollection(nil, []testSignalRow{{name: "a", version: 1}})
	signals := changeSignals(krtutil.NewKrtOptions(stop, nil), "TestSignals", rows.AsCollection())
	signals.WaitUntilSynced(stop)

	first := signals.GetKey("a")
	require.NotNil(t, first, "a row has a signal once synced")

	last := *first
	for version := 2; version <= 4; version++ {
		rows.UpdateObject(testSignalRow{name: "a", version: version})
		require.Eventually(t, func() bool {
			s := signals.GetKey("a")
			return s != nil && !s.Equals(last)
		}, 2*time.Second, 5*time.Millisecond, "every row change changes the signal")
		last = *signals.GetKey("a")
	}

	rows.DeleteObject("a")
	require.Eventually(t, func() bool {
		return signals.GetKey("a") == nil
	}, 2*time.Second, 5*time.Millisecond, "a deleted row's signal is deleted")
}

func TestFetchSignaledNeedsBoth(t *testing.T) {
	ctx := krt.TestingDummyContext{}
	row := testSignalRow{name: "a", version: 1}
	signal := changeSignal{key: "a", n: 1}

	rows := krt.NewStaticCollection(nil, []testSignalRow{row})
	assert.Nil(t, fetchSignaled(ctx, krt.NewStaticCollection[changeSignal](nil, nil), rows, "a"),
		"a row without its signal is not ready yet")

	signals := krt.NewStaticCollection(nil, []changeSignal{signal})
	assert.Nil(t, fetchSignaled(ctx, signals, krt.NewStaticCollection[testSignalRow](nil, nil), "a"),
		"a signal without its row waits for the signal's pending delete or update")

	got := fetchSignaled(ctx, signals, rows, "a")
	require.NotNil(t, got)
	assert.Equal(t, row, *got)
}

// A transform that reads through a signal follows the row: it reruns on every
// change and ends on the latest row, as it would fetching the row itself.
func TestFetchSignaledTransformReadsLatestRow(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	krtopts := krtutil.NewKrtOptions(stop, nil)
	rows := krt.NewMutableCollection(nil, []testSignalRow{{name: "a", version: 1}})
	signals := changeSignals(krtopts, "TestSignals", rows.AsCollection())
	keys := krt.NewStaticCollection(nil, []string{"a"})
	reads := krt.NewCollection(keys, func(kctx krt.HandlerContext, key string) *testSignalRow {
		return fetchSignaled(kctx, signals, rows.AsCollection(), key)
	}, krtopts.ToOptions("TestReads")...)
	reads.WaitUntilSynced(stop)

	readsVersion := func(version int) func() bool {
		return func() bool {
			r := reads.GetKey("a")
			return r != nil && r.version == version
		}
	}
	require.Eventually(t, readsVersion(1), 2*time.Second, 5*time.Millisecond)

	for version := 2; version <= 20; version++ {
		rows.UpdateObject(testSignalRow{name: "a", version: version})
	}
	require.Eventually(t, readsVersion(20), 2*time.Second, 5*time.Millisecond, "the reader ends on the latest row")

	rows.DeleteObject("a")
	require.Eventually(t, func() bool {
		return reads.GetKey("a") == nil
	}, 2*time.Second, 5*time.Millisecond, "a deleted row reruns the reader")

	rows.UpdateObject(testSignalRow{name: "a", version: 21})
	require.Eventually(t, readsVersion(21), 2*time.Second, 5*time.Millisecond, "a re-added row reruns the reader")
}

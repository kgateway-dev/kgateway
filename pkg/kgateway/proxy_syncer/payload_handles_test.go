package proxy_syncer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// testPayloadRow is a minimal row: name is its key and version its content.
type testPayloadRow struct {
	name    string
	version int
	// +noKrtEquals rides along uncompared, to tell equal values apart
	note string
}

func (r testPayloadRow) ResourceName() string { return r.name }

func (r testPayloadRow) Equals(in testPayloadRow) bool {
	return r.name == in.name && r.version == in.version
}

func TestPayloadHandleEquals(t *testing.T) {
	h := payloadHandle{key: "a", version: 1}
	assert.True(t, h.Equals(payloadHandle{key: "a", version: 1}))
	assert.False(t, h.Equals(payloadHandle{key: "a", version: 2}), "a new version is a change")
	assert.False(t, h.Equals(payloadHandle{key: "b", version: 1}), "the key is compared")
}

func TestPayloadCellsVersionFollowsEquals(t *testing.T) {
	cells := newPayloadCells[testPayloadRow]()
	first := cells.store("a", testPayloadRow{name: "a", version: 1, note: "first"})

	same := cells.store("a", testPayloadRow{name: "a", version: 1, note: "second"})
	assert.Equal(t, first, same, "an equal payload keeps the handle, so KRT drops the event")
	payload, version := cells.load("a")
	require.NotNil(t, payload)
	assert.Equal(t, "first", payload.note, "the cell keeps the older equal value, as KRT keeps a stored row")
	assert.Equal(t, first.version, version)

	changed := cells.store("a", testPayloadRow{name: "a", version: 2})
	assert.Greater(t, changed.version, first.version, "a changed payload gets a new version")

	other := cells.store("b", testPayloadRow{name: "b", version: 2})
	assert.NotEqual(t, changed.version, other.version, "versions are unique across keys")

	payload, version = cells.load("missing")
	assert.Nil(t, payload)
	assert.Zero(t, version)
}

func TestPayloadCellsForgetIfAbsent(t *testing.T) {
	cells := newPayloadCells[testPayloadRow]()
	first := cells.store("a", testPayloadRow{name: "a", version: 1})

	cells.forgetIfAbsent("a", func() bool { return false })
	payload, _ := cells.load("a")
	assert.NotNil(t, payload, "a key the parent still has keeps its cell")

	cells.forgetIfAbsent("a", func() bool { return true })
	payload, _ = cells.load("a")
	assert.Nil(t, payload, "a key the parent lost is forgotten")

	readded := cells.store("a", testPayloadRow{name: "a", version: 1})
	assert.Greater(t, readded.version, first.version,
		"a forgotten key's equal payload still gets a new version, so a reader that saw the empty cell reruns")
}

// The cells must not outlive the rows: a departed client's EDS or CDS payload
// would otherwise stay in memory.
func TestPayloadSourceFollowsParentRows(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	rows := krt.NewMutableCollection(nil, []testPayloadRow{{name: "a", version: 1}})
	source := payloadSourceFor(krtutil.NewKrtOptions(stop, nil), "TestHandles", rows.AsCollection())
	source.handles.WaitUntilSynced(stop)

	payload, version := source.fetch(krt.TestingDummyContext{}, "a")
	require.NotNil(t, payload, "the cell is written before the handle is returned")
	assert.Equal(t, 1, payload.version)

	rows.UpdateObject(testPayloadRow{name: "a", version: 2})
	require.Eventually(t, func() bool {
		h := source.handles.GetKey("a")
		return h != nil && h.version != version
	}, 2*time.Second, 5*time.Millisecond, "a changed row changes the handle")
	payload, _ = source.fetch(krt.TestingDummyContext{}, "a")
	assert.Equal(t, 2, payload.version)

	rows.DeleteObject("a")
	require.Eventually(t, func() bool {
		payload, _ := source.cells.load("a")
		return payload == nil && source.handles.GetKey("a") == nil
	}, 2*time.Second, 5*time.Millisecond, "a deleted row's cell is forgotten")
	payload, _ = source.fetch(krt.TestingDummyContext{}, "a")
	assert.Nil(t, payload)

	rows.UpdateObject(testPayloadRow{name: "a", version: 2})
	require.Eventually(t, func() bool {
		payload, _ := source.fetch(krt.TestingDummyContext{}, "a")
		return payload != nil
	}, 2*time.Second, 5*time.Millisecond, "a re-added row is stored again")
}

func TestPayloadSourceForgetsNilPayload(t *testing.T) {
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	rows := krt.NewMutableCollection(nil, []testPayloadRow{{name: "a", version: 1}})
	source := newPayloadSource(krtutil.NewKrtOptions(stop, nil), "TestHandles", rows.AsCollection(),
		func(_ krt.HandlerContext, row testPayloadRow) *testPayloadRow {
			if row.version == 0 {
				return nil
			}
			return &row
		})
	source.handles.WaitUntilSynced(stop)
	payload, _ := source.cells.load("a")
	require.NotNil(t, payload)

	rows.UpdateObject(testPayloadRow{name: "a", version: 0})
	require.Eventually(t, func() bool {
		payload, _ := source.cells.load("a")
		return payload == nil && source.handles.GetKey("a") == nil
	}, 2*time.Second, 5*time.Millisecond, "a nil payload drops both the handle and the cell")
}

package iosnapshot

import (
	"encoding/json"
	"sync"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

type History interface {
	// SetApiSnapshot sets the latest input snapshot
	SetApiSnapshot(latestInput *v1snap.ApiSnapshot)
	// GetInputCopy gets an in-memory copy of the output snapshot for all components.
	GetInputCopy() (map[string]interface{}, error)
	// GetInput gets the input snapshot for all components.
	GetInput() ([]byte, error)

	// SetXdsSnapshotCache sets the cache that is used to store the xDS snapshots
	SetXdsSnapshotCache(cache cache.SnapshotCache)
	// GetXdsSnapshotCache returns the cache that is used to store the xDS snapshots
	GetXdsSnapshotCache() ([]byte, error)
}

func NewHistory() History {
	return &history{
		latestInput: map[string]json.Marshaler{},
		xdsCache:    nil,
	}
}

type history struct {
	sync.RWMutex
	latestInput map[string]json.Marshaler
	xdsCache    cache.SnapshotCache
}

func (h *history) SetApiSnapshot(latestApiSnapshot *v1snap.ApiSnapshot) {
	h.Lock()
	defer h.Unlock()

	h.latestInput["api-snapshot"] = &apiSnapshotJsonMarshaller{
		snap: latestApiSnapshot,
	}
}

func (h *history) GetInput() ([]byte, error) {
	input, err := h.GetInputCopy()
	if err != nil {
		return nil, err
	}

	return formatMap("json_compact", input)
}

func (h *history) GetInputCopy() (map[string]interface{}, error) {
	h.RLock()
	defer h.RUnlock()
	if h.latestInput == nil {
		return map[string]interface{}{}, nil
	}
	genericMaps, err := getGenericMaps(h.latestInput)
	if err != nil {
		return nil, err
	}
	return genericMaps, nil
}

func (h *history) SetXdsSnapshotCache(cache cache.SnapshotCache) {
	h.Lock()
	defer h.Unlock()
	h.xdsCache = cache
}

func (h *history) GetXdsSnapshotCache() ([]byte, error) {
	h.RLock()
	defer h.RUnlock()

	cacheKeys := h.xdsCache.GetStatusKeys()
	cacheEntries := make(map[string]interface{}, len(cacheKeys))

	for _, k := range cacheKeys {
		xdsSnapshot, err := h.xdsCache.GetSnapshot(k)
		if err != nil {
			cacheEntries[k] = err.Error()
		} else {
			cacheEntries[k] = xdsSnapshot
		}
	}

	return formatMap("json_compact", cacheEntries)
}

// apiSnapshotJsonMarshaller is a temporary solution to provide a MarshalJSON interface to the History interface
// Help Wanted: It would be preferable to support a MarshalJSON directly on the ApiSnapshot type
type apiSnapshotJsonMarshaller struct {
	snap *v1snap.ApiSnapshot
}

var _ json.Marshaler = new(apiSnapshotJsonMarshaller)

func (a apiSnapshotJsonMarshaller) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.snap)
}

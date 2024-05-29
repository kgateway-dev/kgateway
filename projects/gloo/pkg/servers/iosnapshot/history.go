package iosnapshot

import (
	"encoding/json"
	"sync"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// History represents an object that maintains state about the running system
// The ControlPlane will use the Setters to update the last known state,
// and the Getters will be used by the Admin Server
type History interface {
	// SetApiSnapshot sets the latest ApiSnapshot
	SetApiSnapshot(latestInput *v1snap.ApiSnapshot)
	// GetInputSnapshotCopy gets an in-memory copy of the output snapshot for all components.
	// Note that this may contain sensitive data and secrets.
	GetInputSnapshotCopy() (map[string]interface{}, error)
	// GetInputSnapshot gets the input snapshot for all components.
	GetInputSnapshot() ([]byte, error)
	// GetXdsSnapshot returns the entire cache of xDS snapshots
	GetXdsSnapshot() ([]byte, error)
}

// NewHistory returns an implementation of the History interface
func NewHistory(cache cache.SnapshotCache) History {
	return &history{
		latestApiSnapshot: nil,
		xdsCache:          cache,
	}
}

type history struct {
	// TODO:
	// 	We rely on a mutex to prevent races reading/writing the data for this object
	//	We should instead use channels to coordinate this
	sync.RWMutex
	latestApiSnapshot *v1snap.ApiSnapshot
	xdsCache          cache.SnapshotCache
}

// SetApiSnapshot sets the latest input ApiSnapshot
func (h *history) SetApiSnapshot(latestApiSnapshot *v1snap.ApiSnapshot) {
	// Setters are called by the running Control Plane, so we perform the update in a goroutine to prevent
	// any contention/issues, from impacting the runtime of the system
	go func() {
		h.setApiSnapshotSafe(latestApiSnapshot)
	}()
}

// setApiSnapshotSafe sets the latest input ApiSnapshot
func (h *history) setApiSnapshotSafe(latestApiSnapshot *v1snap.ApiSnapshot) {
	h.Lock()
	defer h.Unlock()

	h.latestApiSnapshot = latestApiSnapshot
}

// GetInputSnapshot gets the input snapshot for all components.
func (h *history) GetInputSnapshot() ([]byte, error) {
	input, err := h.GetInputSnapshotCopy()
	if err != nil {
		return nil, err
	}

	return formatMap("json_compact", input)
}

// GetInputSnapshotCopy gets an in-memory copy of the output snapshot for all components.
// Note that this may contain sensitive data and secrets.
func (h *history) GetInputSnapshotCopy() (map[string]interface{}, error) {
	h.RLock()
	defer h.RUnlock()
	if h.latestApiSnapshot == nil {
		return map[string]interface{}{}, nil
	}
	genericMaps, err := apiSnapshotToGenericMap(h.latestApiSnapshot)
	if err != nil {
		return nil, err
	}
	return genericMaps, nil
}

// GetXdsSnapshot returns the entire cache of xDS snapshots
func (h *history) GetXdsSnapshot() ([]byte, error) {
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

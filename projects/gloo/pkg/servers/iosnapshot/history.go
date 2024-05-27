package iosnapshot

import (
	"encoding/json"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"sync"
)

type History interface {
	// SetApiSnapshot sets the latest input snapshot
	SetApiSnapshot(latestInput *v1snap.ApiSnapshot)
	// GetInputCopy gets an in-memory copy of the output snapshot for all components.
	GetInputCopy() (map[string]interface{}, error)
	// GetInput gets the input snapshot for all components.
	GetInput() ([]byte, error)
}

func NewHistory() History {
	return &history{
		latestInput: map[string]json.Marshaler{},
	}
}

type history struct {
	sync.RWMutex
	latestInput map[string]json.Marshaler
}

func (h *history) SetApiSnapshot(latestApiSnapshot *v1snap.ApiSnapshot) {
	h.Lock()
	defer h.Unlock()

	snapshotMarshaller := &apiSnapshotJsonMarshaller{
		snap: latestApiSnapshot,
	}
	h.latestInput["api-snapshot"] = snapshotMarshaller
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

type apiSnapshotJsonMarshaller struct {
	snap *v1snap.ApiSnapshot
}

func (a apiSnapshotJsonMarshaller) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.snap)
}

var _ json.Marshaler = new(apiSnapshotJsonMarshaller)

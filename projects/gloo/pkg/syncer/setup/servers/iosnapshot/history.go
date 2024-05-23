package iosnapshot

import (
	"encoding/json"
	"sync"
)

type History interface {
	// SetApiSnapshot sets the latest input snapshot
	SetApiSnapshot(latestInput json.Marshaler)
	// GetInputCopy gets an in-memory copy of the output snapshot
	GetInputCopy() (map[string]interface{}, error)
	// GetFilteredInput gets the input snapshot applies filters
	GetFilteredInput(format string, filters Filters) ([]byte, error)
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

func (h *history) SetApiSnapshot(latestApiSnapshot json.Marshaler) {
	h.Lock()
	defer h.Unlock()
	h.latestInput["api-snapshot"] = latestApiSnapshot
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

func (h *history) GetFilteredInput(format string, filters Filters) ([]byte, error) {
	input, err := h.GetInputCopy()
	if err != nil {
		return nil, err
	}

	// short circuit if no filters
	if !filters.namespaces.Exists() && !filters.resourceTypes.Exists() {
		return formatMap(format, input)
	}

	// TODO: respect namesapces in filterMap
	return formatMap(format, input)
}

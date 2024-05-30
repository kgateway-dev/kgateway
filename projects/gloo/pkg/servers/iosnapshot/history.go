package iosnapshot

import (
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
	// GetRedactedApiSnapshot gets an in-memory copy of the ApiSnapshot
	// Any sensitive data contained in the Snapshot will either be explicitly redacted
	// or entirely excluded
	GetRedactedApiSnapshot() (map[string]interface{}, error)
	// GetInputSnapshot gets the input snapshot for all components.
	GetInputSnapshot() ([]byte, error)
	// GetProxySnapshot returns the Proxies generated for all components.
	GetProxySnapshot() ([]byte, error)
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

	// To ensure that any modifications we perform on the ApiSnapshot DO NOT impact the Control Plane
	clonedSnapshot := latestApiSnapshot.Clone()

	h.latestApiSnapshot = &clonedSnapshot
}

// GetInputSnapshot gets the input snapshot for all components.
func (h *history) GetInputSnapshot() ([]byte, error) {
	input, err := h.GetRedactedApiSnapshot()
	if err != nil {
		return nil, err
	}

	// todo: remove proxies from what is returned?

	return formatMap("json_compact", input)
}

func (h *history) GetProxySnapshot() ([]byte, error) {
	input, err := h.GetRedactedApiSnapshot()
	if err != nil {
		return nil, err
	}

	// todo: remove all types EXCEPT proxies

	return formatMap("json_compact", input)
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

// GetRedactedApiSnapshot gets an in-memory copy of the ApiSnapshot
// Any sensitive data contained in the Snapshot will either be explicitly redacted
// or entirely excluded
// NOTE: Redaction is somewhat of an expensive operation, so we have a few options for how to approach it:
//
//  1. Perform it when a new ApiSnapshot is received from the Control Plane
//
//  2. Perform it on demand, when an ApiSnapshot is requested
//
//  3. Perform it on demand, when an ApiSnapshot is requested, but store a local cache for future requests.
//     That cache would be invalidated each time a new ApiSnapshot is received.
//
//     Given that the rate of requests for the ApiSnapshot <<< the frequency of updates of an ApiSnapshot by the Control Plane,
//     in this first pass we opt to take approach #2.
func (h *history) GetRedactedApiSnapshot() (map[string]interface{}, error) {
	if h.latestApiSnapshot == nil {
		return map[string]interface{}{}, nil
	}

	redactedSnapshot := redactApiSnapshot(h.latestApiSnapshot)
	genericMaps, err := apiSnapshotToGenericMap(redactedSnapshot)
	if err != nil {
		return nil, err
	}
	return genericMaps, nil
}

// redactApiSnapshot accepts an ApiSnapshot, and returns a cloned representation of that Snapshot,
// but without any sensitive data. It is critical that data which is exposed by this component
// is redacted, so that customers can feel comfortable sharing the results with us.
//
// NOTE: This is an extremely naive implementation. It is intended as a first pass to get this API
// into the hands of the field.As we iterate on this component, we can use some of the redaction
// utilities in `/pkg/utils/syncutil`.
func redactApiSnapshot(original *v1snap.ApiSnapshot) *v1snap.ApiSnapshot {
	redacted := original.Clone()

	redacted.Secrets = nil

	// See `pkg/utils/syncutil/log_redactor.StringifySnapshot` for an explanation for
	// why we redact Artifacts
	redacted.Artifacts = nil

	return &redacted
}

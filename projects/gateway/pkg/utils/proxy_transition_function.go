package utils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/hashutils"
)

func TransitionFunction(original, desired *v1.Proxy) (bool, error) {
	if len(original.GetListeners()) != len(desired.GetListeners()) {
		updateDesiredStatus(original, desired)
		return true, nil
	}
	for i := range original.GetListeners() {
		if !original.GetListeners()[i].Equal(desired.GetListeners()[i]) {
			updateDesiredStatus(original, desired)
			return true, nil
		}
	}
	return false, nil
}

func updateDesiredStatus(original, desired *v1.Proxy) {
	// we made an update to the proxy from the gateway's point of view.
	// let's make sure we update the status for gloo if the hash hasn't changed.
	// the proxy can change from the gateway's point of view but not from gloo's if,
	// for example, the generation changes on a listener.
	//
	// this is sort of a hack around using subresource statuses for the proxy status
	// until we make the full move.
	equal, ok := hashutils.HashableEqual(original, desired)
	if ok && equal {
		desired.Status = original.Status
	}
}

package utils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/hashutils"
)

func TransitionFunction(original, desired *v1.Proxy) (bool, error) {
	// this logic needs to match the resync equality for gloo, or else
	// we can get into a scenario where gateway reconciles a new proxy that
	// has the same hash as the previous, but with a pending status, that never
	// gets cleared because gloo is waiting for a "significant" change before resyncing.
	equal, ok := hashutils.HashableEqual(original, desired)
	if ok {
		return !equal, nil
	}
	// default behavior: perform the update if one if the objects are not hashable
	return true, nil
}

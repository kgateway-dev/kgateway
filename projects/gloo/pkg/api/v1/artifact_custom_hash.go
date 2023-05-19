package v1

import (
	"encoding/binary"
	"hash"
	"hash/fnv"

	safe_hasher "github.com/solo-io/protoc-gen-ext/pkg/hasher"
	"github.com/solo-io/protoc-gen-ext/pkg/hasher/hashstructure"
)

// This is a custom implementation of the `SafeHasher` interface for Artifacts.
// If works just as its generated counterpart, except that it includes `ResourceVersion` instead of `Data` in the hash.
func (m *Artifact) Hash(hasher hash.Hash64) (uint64, error) {
	if hasher == nil {
		hasher = fnv.New64()
	}
	var err error
	if _, err = hasher.Write([]byte("gloo.solo.io.github.com/solo-io/gloo/projects/gloo/pkg/api/v1.Artifact")); err != nil {
		return 0, err
	}

	// Instead using "data" (which might be expensive in case of large config maps), always look at the resource version
	if _, err = hasher.Write([]byte(m.GetMetadata().GetResourceVersion())); err != nil {
		return 0, err
	}

	if h, ok := interface{}(&m.Metadata).(safe_hasher.SafeHasher); ok {
		if _, err = h.Hash(hasher); err != nil {
			return 0, err
		}
	} else {
		if val, err := hashstructure.Hash(&m.Metadata, nil); err != nil {
			return 0, err
		} else {
			if err := binary.Write(hasher, binary.LittleEndian, val); err != nil {
				return 0, err
			}
		}
	}

	return hasher.Sum64(), nil
}

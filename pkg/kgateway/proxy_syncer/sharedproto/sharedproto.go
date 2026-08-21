// Package sharedproto makes the xDS protos that kgateway shares across
// per-client snapshots immutable by construction. The base+overlay split
// aliases Cluster and ClusterLoadAssignment protos across clients (shared
// bases, interned per-client deltas, interned CLAs); a post-creation mutation
// corrupts every sibling client's snapshot plus the copy stored in KRT, and is
// invisible to KRT equality because version hashes are computed at store time.
//
// Shared[M] holds the proto in an unexported field of this package, so
// consumer code in proxy_syncer cannot reach the pointer at all: the only ways
// out are ResourceWithTTL (which hands it to the envoycache snapshot, the one
// legitimate sink, verifying the tripwire on the way) and Clone (the one
// legitimate mutation path). The remaining seam is deliberate and greppable: a
// caller can type-assert ResourceWithTTL().Resource back to the concrete
// proto, but cannot do so by accident.
package sharedproto

import (
	"fmt"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
)

// AssertImmutability arms the mutation tripwire: Wrap captures each proto's
// content hash at wrap time and ResourceWithTTL re-hashes at snapshot-assembly
// time, panicking on drift. Off by default in production — the re-hash is a
// full deterministic marshal per resource per snapshot rebuild, which is
// exactly the cost the interning exists to avoid.
//
// Enabled in CI three ways: the proxy_syncer package tests force it on
// in-process (TestMain), the e2e suites set ASSERT_SHARED_PROTO_IMMUTABILITY
// on the deployed controller via common-recommendations.yaml, and the
// conformance action sets it on both of its helm install branches. A trip in a
// cluster surfaces as a controller panic/restart; the message is in the
// previous container's logs (kubectl logs --previous).
var AssertImmutability = envutils.IsEnvTruthy("ASSERT_SHARED_PROTO_IMMUTABILITY")

// Shared wraps a proto that is aliased across per-client xDS snapshots.
// The zero value is an empty wrapper: IsNil reports true and verification is
// disabled (hash 0 = "not captured"), which is what rows built without a proto
// (e.g. status-only views and test fixtures) get for free.
type Shared[M proto.Message] struct {
	msg M
	// hash is the content hash captured at wrap time when AssertImmutability
	// was set; 0 means "not captured" and disables verification.
	hash uint64
}

// Wrap takes ownership of msg as a shared, read-only proto. The caller must
// not retain or mutate msg after wrapping; hand out copies via Clone.
func Wrap[M proto.Message](msg M) Shared[M] {
	var hash uint64
	if AssertImmutability {
		hash = utils.HashProto(msg)
	}
	return Shared[M]{msg: msg, hash: hash}
}

// WrapPrehashed is Wrap for producers that already computed the proto's
// utils.HashProto content hash (e.g. for versioning), making capture free.
// Pass 0 to explicitly opt the proto out of verification (e.g. error-path
// blackholes that are never published).
func WrapPrehashed[M proto.Message](msg M, contentHash uint64) Shared[M] {
	if !AssertImmutability {
		contentHash = 0
	}
	return Shared[M]{msg: msg, hash: contentHash}
}

// IsNil reports whether the wrapper carries no proto (zero value or wrapped
// nil pointer).
func (s Shared[M]) IsNil() bool {
	return any(s.msg) == nil || !s.msg.ProtoReflect().IsValid()
}

// Clone returns a deep copy the caller owns and may mutate. This is the only
// way to derive a mutable proto from a shared one.
func (s Shared[M]) Clone() M {
	return proto.Clone(s.msg).(M)
}

// ResourceWithTTL hands the shared proto to the envoycache snapshot — the one
// legitimate sink for the raw pointer. When AssertImmutability is armed it
// first re-hashes the proto and panics if it no longer matches its wrap-time
// hash, naming the resource.
func (s Shared[M]) ResourceWithTTL() envoycachetypes.ResourceWithTTL {
	if AssertImmutability && s.hash != 0 {
		if got := utils.HashProto(s.msg); got != s.hash {
			panic(fmt.Sprintf(
				"shared proto %q (%s) was mutated after creation (hash %d at wrap, %d now): "+
					"protos wrapped in sharedproto.Shared are aliased across client snapshots and MUST NOT be mutated — use Clone",
				resourceLabel(s.msg), s.msg.ProtoReflect().Descriptor().FullName(), s.hash, got))
		}
	}
	return envoycachetypes.ResourceWithTTL{Resource: s.msg}
}

// Same reports whether two wrappers alias the same underlying proto instance.
// Intended for tests asserting interning/sharing behavior.
func Same[M proto.Message](a, b Shared[M]) bool {
	return any(a.msg) == any(b.msg)
}

// Is reports whether the wrapper aliases exactly msg. Intended for tests that
// hold the raw proto they handed to Wrap.
func (s Shared[M]) Is(msg M) bool {
	return any(s.msg) == any(msg)
}

// resourceLabel best-effort names a resource for the tripwire panic message.
func resourceLabel(m proto.Message) string {
	switch v := any(m).(type) {
	case interface{ GetName() string }:
		if n := v.GetName(); n != "" {
			return n
		}
	}
	if v, ok := any(m).(interface{ GetClusterName() string }); ok {
		if n := v.GetClusterName(); n != "" {
			return n
		}
	}
	return "<unnamed>"
}

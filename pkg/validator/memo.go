package validator

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"google.golang.org/protobuf/proto"
)

// ContentKey identifies a proto by content: a SHA-256 over its deterministic
// binary encoding. Two messages with the same key are, for validation purposes,
// the same message.
type ContentKey [sha256.Size]byte

// ContentKeyOf returns the ContentKey of m. It marshals m once with map keys
// sorted, so the key is stable across recomputes of an equal message; it does
// not clone, build a bootstrap, or render JSON, which is what makes it cheap
// enough to compute on every validation attempt (about 1 µs for a typical
// cluster, versus ~20 µs for the bootstrap-level cache key).
func ContentKeyOf(m proto.Message) (ContentKey, error) {
	var buf [4096]byte
	out, err := proto.MarshalOptions{Deterministic: true}.MarshalAppend(buf[:0], m)
	if err != nil {
		return ContentKey{}, err
	}
	return sha256.Sum256(out), nil
}

// Memo memoizes validation verdicts by ContentKey, in front of whatever
// Validator the caller runs on a miss. It exists for callers that validate the
// same resource many times over — the per-client backend translation validates
// each overlaid cluster once per connected client on every walk — and for whom
// even the bootstrap-level cache in cachingValidator is too expensive to reach,
// because reaching it means building a bootstrap and hashing its JSON.
//
// A verdict is a pure function of the resource bytes, so memoizing it cannot
// change an outcome. Like cachingValidator, only success and ErrInvalidXDS are
// memoized; transient errors (exec failure, cancellation) run again next time.
// A nil *Memo is valid and memoizes nothing.
type Memo struct {
	cache *lru.Cache[ContentKey, cachedResult]
}

// NewMemo returns a Memo holding up to size verdicts. If size <= 0,
// DefaultCacheSize is used.
func NewMemo(size int) *Memo {
	if size <= 0 {
		size = DefaultCacheSize
	}
	cache, err := lru.New[ContentKey, cachedResult](size)
	if err != nil {
		// lru.New only errors when size <= 0, which is guarded above.
		return nil
	}
	return &Memo{cache: cache}
}

// Validate returns the memoized verdict for key, or runs validate and memoizes
// what it returns. validate receives ctx unchanged; it is expected to carry
// the validation caller (see WithValidationCaller) so that both a memo hit
// here and the inner validator's own metrics attribute to the same caller.
func (m *Memo) Validate(ctx context.Context, key ContentKey, validate func(context.Context) error) error {
	if m == nil {
		return validate(ctx)
	}
	if hit, ok := m.cache.Get(key); ok {
		start := time.Now()
		caller := validationCaller(ctx)
		recordValidationCall(caller)
		recordValidationCacheHit(caller)
		recordValidationResult(caller, validationResultFromError(hit.err()), start)
		return hit.err()
	}
	// A miss records nothing here: the inner validator records the call and
	// its result, and when it is a cachingValidator, its own hit or miss.
	err := validate(ctx)
	switch {
	case err == nil:
		m.cache.Add(key, cachedResult{ok: true})
	case errors.Is(err, ErrInvalidXDS):
		m.cache.Add(key, cachedResult{msg: err.Error()})
	}
	return err
}

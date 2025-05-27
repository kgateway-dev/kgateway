package policy

import (
	"errors"
	"reflect"
)

var ErrUnsupportedMergeStrategy = errors.New("unsupported merge strategy")

// MergeStrategy defines how two policies should be merged
type MergeStrategy string

const (
	// AtomicMerge implies that Merge(p1,p2) will produce p1 or p2, not a composition of both
	AtomicMerge MergeStrategy = "Atomic"

	// AugmentedMerge implies that Merge(p1,p2) will produce a policy composition of p1 and p2 such
	// that the policy contains all set fields in p1 and set fields in p2 that are not set in p1,
	// i.e., p1 is augmented by p2.
	AugmentedMerge MergeStrategy = "Augmented"

	// OverridableMerge implies that Merge(p1,p2) will produce a policy composition of p1 and p2 such
	// that the policy contains all set fields in p2 and set fields in p1 that are not set in p2,
	// i.e., p2 overrides p1.
	OverridableMerge MergeStrategy = "Overridable"
)

type MergeOptions struct {
	// Merge strategy to use
	// Defaults to AugmentedMerge
	Strategy MergeStrategy
}

func IsMergeable(p1, p2 any, opts MergeOptions) bool {
	switch opts.Strategy {
	case AugmentedMerge:
		return isNil(p1) && !isNil(p2)

	case OverridableMerge:
		return !isNil(p2)

	default:
		return false
	}
}

func isNil(arg any) bool {
	if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Ptr ||
		v.Kind() == reflect.Interface ||
		v.Kind() == reflect.Slice ||
		v.Kind() == reflect.Map ||
		v.Kind() == reflect.Chan ||
		v.Kind() == reflect.Func) && v.IsNil()) {
		return true
	}
	return false
}

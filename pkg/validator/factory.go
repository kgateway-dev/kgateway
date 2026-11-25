package validator

import (
	"fmt"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// New constructs a Validator according to the given settings. The default
// (mode=CACHE) wraps the binary validator with a content-hash result cache, a
// pure memoization that cannot change verdicts.
func New(s apisettings.Settings) (Validator, error) {
	base := NewBinary()
	switch s.ValidatorMode {
	case apisettings.ValidatorBinary:
		return base, nil
	case apisettings.ValidatorCache:
		return NewCaching(base, s.ValidatorCacheSize), nil
	default:
		return nil, fmt.Errorf("invalid validator mode: %q", s.ValidatorMode)
	}
}

// BootstrapValidatorFor returns v when it can validate proxy bootstraps, and
// otherwise the Envoy binary behind a result cache. A caching validator can validate
// them only when the validator it wraps can.
func BootstrapValidatorFor(v Validator) BootstrapValidator {
	switch bv := v.(type) {
	case *cachingValidator:
		if _, ok := bv.inner.(BootstrapValidator); ok {
			return bv
		}
	case BootstrapValidator:
		return bv
	}
	return NewCaching(NewBinary(), 0).(BootstrapValidator)
}

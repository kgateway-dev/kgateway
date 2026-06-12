package validator

import (
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// New constructs a Validator according to the given settings. The default
// (mode=CACHE) wraps the binary validator with a content-hash result cache —
// a pure memoization that cannot change verdicts. Unknown modes fall back to
// plain BINARY so misconfiguration cannot block startup.
func New(s apisettings.Settings) Validator {
	base := NewBinary()
	switch s.ValidatorMode {
	case apisettings.ValidatorCache:
		return NewCaching(base, s.ValidatorCacheSize)
	default:
		return base
	}
}

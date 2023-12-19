package rules

import "github.com/quasilyte/go-ruleguard/dsl"

// erisErrorfAsWrapper detects situations where eris.Errorf() is used to wrap an
// error. It then reports a problem and suggests replacing the call with
// eris.Wrap() or eris.Wrapf() instead. Properly wrapped errors allow checking
// for the nested error using errors.Is() and errors.As().
func erisErrorfAsWrapper(m dsl.Matcher) {
	m.Match("eris.Errorf($s, $*_, $x, $*_)").
		Where(m["x"].Type.Implements("error")).
		Report("eris.Errorf() is for creating new errors, use eris.Wrap() or eris.Wrapf() to wrap errors")
}

//go:build ignore

package rules

import "github.com/quasilyte/go-ruleguard/dsl"

func consistentlyTimeout(m dsl.Matcher) {
	// Match `Consistently(func(){}, Timeout(1), ...)` calls
	m.Match(`Consistently($*_, Timeout($_), $*_)`, `$x.Consistently($*_, Timeout($_), $*_)`).
		Report("Timeout() should not be used as an argument within Consistently() call, as this is set to 5m in CI, so it will try run the assertion for a full 5 minutes on the happy path")
}

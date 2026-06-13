package equalstest_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// fixture is a tiny struct used exclusively for testing the harness itself.
type fixture struct {
	A string
	B int
	// C is deliberately ignored by fixtureEqualsMissingC to simulate a bug.
	C string
}

// fixtureEqualsMissingC is a buggy Equals that misses field C.
func fixtureEqualsMissingC(a, b fixture) bool {
	return a.A == b.A && a.B == b.B
}

// fixtureEqualsCorrect compares all fields.
func fixtureEqualsCorrect(a, b fixture) bool {
	return a.A == b.A && a.B == b.B && a.C == b.C
}

func baseFixture() fixture {
	return fixture{A: "hello", B: 42, C: "world"}
}

// TestHarnessSelfTest_CorrectEquals verifies that a complete case list over a
// correct Equals implementation passes without any failures.
func TestHarnessSelfTest_CorrectEquals(t *testing.T) {
	cases := []equalstest.Case[fixture]{
		{
			Field:  "A",
			Mutate: func(f *fixture) { f.A = "changed" },
		},
		{
			Field:  "B",
			Mutate: func(f *fixture) { f.B = 99 },
		},
		{
			Field:  "C",
			Mutate: func(f *fixture) { f.C = "changed" },
		},
	}
	equalstest.Run(t, baseFixture, fixtureEqualsCorrect, cases, nil)
}

// TestHarnessSelfTest_ExemptField verifies that listing an uncovered field as
// exempt prevents the completeness failure.
func TestHarnessSelfTest_ExemptField(t *testing.T) {
	cases := []equalstest.Case[fixture]{
		{
			Field:  "A",
			Mutate: func(f *fixture) { f.A = "changed" },
		},
		{
			Field:  "B",
			Mutate: func(f *fixture) { f.B = 99 },
		},
	}
	// C is listed as exempt; the completeness check must pass without a C Case.
	equalstest.Run(t, baseFixture, fixtureEqualsCorrect, cases, []string{"C"})
}

// TestHarnessSelfTest_PointerToStruct verifies the harness accepts a
// pointer-to-struct type parameter, dereferencing it for the completeness check.
func TestHarnessSelfTest_PointerToStruct(t *testing.T) {
	base := func() *fixture {
		f := baseFixture()
		return &f
	}
	equals := func(a, b *fixture) bool { return fixtureEqualsCorrect(*a, *b) }
	cases := []equalstest.Case[*fixture]{
		{
			Field:  "A",
			Mutate: func(f **fixture) { (*f).A = "changed" },
		},
		{
			Field:  "B",
			Mutate: func(f **fixture) { (*f).B = 99 },
		},
		{
			Field:  "C",
			Mutate: func(f **fixture) { (*f).C = "changed" },
		},
	}
	equalstest.Run(t, base, equals, cases, nil)
}

// TestHarnessSelfTest_FixtureSanity validates the fixture functions themselves
// so that higher-level tests can rely on them. In particular it confirms that
// fixtureEqualsMissingC genuinely misses field C (modelling the bug) and that
// fixtureEqualsCorrect catches it.
func TestHarnessSelfTest_FixtureSanity(t *testing.T) {
	t.Run("buggy_equals_misses_C", func(t *testing.T) {
		orig := baseFixture()
		mutated := baseFixture()
		mutated.C = "changed"
		if !fixtureEqualsMissingC(orig, mutated) {
			t.Error("expected fixtureEqualsMissingC to miss the C change, but it detected it")
		}
	})

	t.Run("correct_equals_detects_C", func(t *testing.T) {
		orig := baseFixture()
		mutated := baseFixture()
		mutated.C = "changed"
		if fixtureEqualsCorrect(orig, mutated) {
			t.Error("expected fixtureEqualsCorrect to detect change in C, but it returned true")
		}
	})
}

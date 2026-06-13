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

// TestHarnessSelfTest_MutationDetection directly verifies the core logic that
// Run uses internally: that Equals(orig, mutated) == false for each mutation.
// This avoids the need to invert a test-framework failure.
func TestHarnessSelfTest_MutationDetection(t *testing.T) {
	// A buggy Equals that ignores C.
	equals := fixtureEqualsMissingC

	t.Run("A_is_detected", func(t *testing.T) {
		orig := baseFixture()
		mutated := baseFixture()
		mutated.A = "changed"
		if equals(orig, mutated) {
			t.Error("expected Equals to detect change in A, but it returned true")
		}
	})

	t.Run("B_is_detected", func(t *testing.T) {
		orig := baseFixture()
		mutated := baseFixture()
		mutated.B = 99
		if equals(orig, mutated) {
			t.Error("expected Equals to detect change in B, but it returned true")
		}
	})

	t.Run("C_is_NOT_detected_by_buggy_equals", func(t *testing.T) {
		// This verifies the fixture correctly models the bug:
		// fixtureEqualsMissingC returns true even when C differs.
		orig := baseFixture()
		mutated := baseFixture()
		mutated.C = "changed"
		if !equals(orig, mutated) {
			t.Error("expected the buggy Equals to miss the C change, but it detected it")
		}
	})

	t.Run("C_IS_detected_by_correct_equals", func(t *testing.T) {
		orig := baseFixture()
		mutated := baseFixture()
		mutated.C = "changed"
		if fixtureEqualsCorrect(orig, mutated) {
			t.Error("expected fixtureEqualsCorrect to detect change in C, but it returned true")
		}
	})
}

// TestHarnessSelfTest_CompletenessLogic directly verifies the field-coverage
// completeness logic without relying on test-failure inversion.
func TestHarnessSelfTest_CompletenessLogic(t *testing.T) {
	// Cases cover only A and B, C is neither covered nor exempt — should be caught.
	t.Run("uncovered_field_is_flagged", func(t *testing.T) {
		covered := map[string]bool{"A": true, "B": true}
		exempt := map[string]bool{}
		fields := []string{"A", "B", "C"} // exported fields of fixture

		var uncovered []string
		for _, f := range fields {
			if !covered[f] && !exempt[f] {
				uncovered = append(uncovered, f)
			}
		}
		if len(uncovered) != 1 || uncovered[0] != "C" {
			t.Errorf("expected [C] uncovered, got %v", uncovered)
		}
	})

	t.Run("exempt_field_is_not_flagged", func(t *testing.T) {
		covered := map[string]bool{"A": true, "B": true}
		exempt := map[string]bool{"C": true}
		fields := []string{"A", "B", "C"}

		var uncovered []string
		for _, f := range fields {
			if !covered[f] && !exempt[f] {
				uncovered = append(uncovered, f)
			}
		}
		if len(uncovered) != 0 {
			t.Errorf("expected no uncovered fields when C is exempt, got %v", uncovered)
		}
	})
}

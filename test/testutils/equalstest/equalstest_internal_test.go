// Package equalstest white-box tests for unexported helpers.
package equalstest

import (
	"reflect"
	"testing"
)

// EmbeddedBase is an exported struct embedded inside internalFixture to test
// that uncoveredFields flattens one level of anonymous embedding.
type EmbeddedBase struct {
	Embedded string
}

// internalFixture is a local fixture struct with a plain field and an exported
// embedded struct, used exclusively for testing uncoveredFields.
type internalFixture struct {
	Plain string
	EmbeddedBase
}

func TestUncoveredFields_UncoveredFieldIsFlagged(t *testing.T) {
	typ := reflect.TypeFor[internalFixture]()
	// Cover Plain and the embedding name EmbeddedBase, but not the flattened field Embedded.
	covered := map[string]bool{"Plain": true, "EmbeddedBase": true}
	exempt := map[string]bool{}

	// "Embedded" comes from flattening EmbeddedBase; it is not in covered or exempt.
	missing := uncoveredFields(typ, covered, exempt)
	if len(missing) != 1 || missing[0] != "Embedded" {
		t.Errorf("expected [Embedded] to be uncovered, got %v", missing)
	}
}

func TestUncoveredFields_ExemptFieldIsNotFlagged(t *testing.T) {
	typ := reflect.TypeFor[internalFixture]()
	covered := map[string]bool{"Plain": true, "EmbeddedBase": true}
	exempt := map[string]bool{"Embedded": true}

	missing := uncoveredFields(typ, covered, exempt)
	if len(missing) != 0 {
		t.Errorf("expected no uncovered fields when Embedded is exempt, got %v", missing)
	}
}

func TestUncoveredFields_EmbeddedStructNameAlsoReturned(t *testing.T) {
	typ := reflect.TypeFor[internalFixture]()
	// Cover the flattened field but leave the embedding name itself uncovered.
	covered := map[string]bool{"Plain": true, "Embedded": true}
	exempt := map[string]bool{}

	// exportedFields emits both "Embedded" (flattened) and "EmbeddedBase" (the
	// embedding name itself). With covered containing only "Embedded",
	// "EmbeddedBase" must appear as uncovered.
	missing := uncoveredFields(typ, covered, exempt)
	if len(missing) != 1 || missing[0] != "EmbeddedBase" {
		t.Errorf("expected [EmbeddedBase] to be uncovered, got %v", missing)
	}
}

func TestUncoveredFields_AllCoveredReturnsEmpty(t *testing.T) {
	typ := reflect.TypeFor[internalFixture]()
	// Cover every name that exportedFields produces for internalFixture.
	covered := map[string]bool{"Plain": true, "Embedded": true, "EmbeddedBase": true}
	exempt := map[string]bool{}

	missing := uncoveredFields(typ, covered, exempt)
	if len(missing) != 0 {
		t.Errorf("expected no uncovered fields, got %v", missing)
	}
}

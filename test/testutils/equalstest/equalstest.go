// Package equalstest verifies that an IR type's Equals method detects a
// change in every exported field, so KRT collections never miss an update.
package equalstest

import (
	"fmt"
	"reflect"
	"testing"
)

// Option configures the behavior of Run.
type Option func(*config)

type config struct {
	includeUnexported bool
}

// IncludeUnexported makes the completeness check consider unexported fields in
// addition to exported ones. Use it for IR types whose fields are all
// unexported (e.g. plugin-internal PolicyIRs), where the default
// exported-only check would enforce nothing. The test must live in the same
// package as T so its Mutate closures can reach those fields.
func IncludeUnexported() Option {
	return func(c *config) { c.includeUnexported = true }
}

// Case mutates one logical field of T and states whether Equals must
// report inequality afterwards.
type Case[T any] struct {
	// Field is the exported Go field name this case covers (e.g. "Listeners").
	// Used to satisfy the completeness check.
	Field string
	// Mutate changes that field on the given instance.
	Mutate func(*T)
}

// Run builds two fresh instances via base() for each case, applies
// Mutate to one, and asserts:
//  1. base().Equals(base()) is true (reflexivity on identical values)
//  2. after mutation, Equals is false in both directions (detection + symmetry)
//
// It then reflects over T's exported fields (flattening embedded structs one
// level) and fails if any field name is neither covered by a Case nor listed
// in exempt — that is how "new field, forgot Equals" becomes a test failure.
//
// T must be a struct or a pointer to a struct; pointers are dereferenced one
// level before field reflection.
func Run[T any](t *testing.T, base func() T, equals func(a, b T) bool, cases []Case[T], exempt []string, opts ...Option) {
	t.Helper()

	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	// 1. Reflexivity check: two identical base instances must be equal.
	t.Run("reflexivity", func(t *testing.T) {
		t.Helper()
		a := base()
		b := base()
		if !equals(a, b) {
			t.Errorf("Equals(base(), base()) returned false; two identical instances must be equal")
		}
	})

	// 2. Mutation cases: each mutation must cause Equals to return false.
	for _, c := range cases {
		t.Run("mutation/"+c.Field, func(t *testing.T) {
			t.Helper()
			orig := base()
			mutated := base()
			c.Mutate(&mutated)

			if equals(orig, mutated) {
				t.Errorf("Equals returned true after mutating field %q; Equals must detect this change", c.Field)
			}
			// Symmetry: a.Equals(b) must equal b.Equals(a)
			if equals(mutated, orig) {
				t.Errorf("Equals is not symmetric: Equals(orig, mutated)=false but Equals(mutated, orig)=true for field %q", c.Field)
			}
		})
	}

	// 3. Completeness check: every exported field of T must appear in a Case or in exempt.
	covered := make(map[string]bool, len(cases))
	for _, c := range cases {
		covered[c.Field] = true
	}
	exemptSet := make(map[string]bool, len(exempt))
	for _, e := range exempt {
		exemptSet[e] = true
	}

	typ := reflect.TypeFor[T]()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		t.Fatalf("equalstest.Run: type %s is not a struct or pointer to struct", typ)
	}

	missing := uncoveredFields(typ, covered, exemptSet, cfg.includeUnexported)
	if len(missing) > 0 {
		t.Errorf(
			"completeness check failed for %s: exported field(s) %v are neither covered by a mutation Case nor listed as exempt — add a Case or add the field name to exempt",
			typeName(typ),
			missing,
		)
	}
}

// uncoveredFields returns the exported field names of typ that are not present
// in covered or exempt. It flattens anonymous (embedded) struct fields one
// level deep, matching the same logic used by Run's completeness check.
func uncoveredFields(typ reflect.Type, covered map[string]bool, exempt map[string]bool, includeUnexported bool) []string {
	var missing []string
	for _, field := range candidateFields(typ, includeUnexported) {
		if !covered[field] && !exempt[field] {
			missing = append(missing, field)
		}
	}
	return missing
}

// candidateFields returns the field names of a struct type that the
// completeness check should cover, flattening anonymous (embedded) struct
// fields one level deep. Unexported fields are included only when
// includeUnexported is set.
func candidateFields(t reflect.Type, includeUnexported bool) []string {
	var names []string
	for f := range t.Fields() {
		if !f.IsExported() && !includeUnexported {
			continue
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			// Flatten embedded struct fields one level.
			embedded := f.Type
			for ef := range embedded.Fields() {
				if ef.IsExported() || includeUnexported {
					names = append(names, ef.Name)
				}
			}
			// Also add the embedded type's own name so the test can explicitly
			// target the whole embedding as a single field (e.g. "ObjectSource").
			names = append(names, f.Name)
			continue
		}
		names = append(names, f.Name)
	}
	return names
}

func typeName(t reflect.Type) string {
	if t.PkgPath() != "" {
		return fmt.Sprintf("%s.%s", t.PkgPath(), t.Name())
	}
	return t.Name()
}

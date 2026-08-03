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
//  2. Mutate actually changed the value, so a broken mutator is reported as such
//     instead of being blamed on Equals
//  3. after mutation, Equals is false, and agrees in both directions (detection +
//     symmetry)
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

			// Precondition: Mutate must actually have changed something, otherwise
			// the case below would blame Equals for a broken mutator. DeepEqual is
			// used only to detect "nothing changed at all": over-reporting a
			// difference (as it may for independently built protos) is harmless
			// here, and it never reports two genuinely different values as equal.
			if !mutationChanged(orig, mutated) {
				t.Fatalf(
					"Mutate for field %q left the value unchanged, so this case tests nothing; "+
						"fix the mutator (or the base() factory, if it now returns the mutated value)",
					c.Field,
				)
			}

			fwd := equals(orig, mutated)
			rev := equals(mutated, orig)
			if fwd != rev {
				t.Errorf(
					"Equals is not symmetric for field %q: Equals(orig, mutated)=%v but Equals(mutated, orig)=%v",
					c.Field, fwd, rev,
				)
			}
			if fwd {
				t.Errorf("Equals returned true after mutating field %q; Equals must detect this change", c.Field)
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

// mutationChanged reports whether Mutate actually altered the value. DeepEqual is
// used only as a "nothing changed at all" detector: for types holding
// independently built protos it may report a difference that proto.Equal would
// call equal, which is harmless here, and it never reports two genuinely
// different values as identical.
func mutationChanged(orig, mutated any) bool {
	return !reflect.DeepEqual(orig, mutated)
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

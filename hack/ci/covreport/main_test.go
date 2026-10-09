package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const unitProfile = `mode: atomic
github.com/kgateway-dev/kgateway/v2/pkg/a/a.go:10.1,12.2 2 1
github.com/kgateway-dev/kgateway/v2/pkg/a/a.go:14.1,16.2 3 0
`

const e2eProfile = `mode: atomic
github.com/kgateway-dev/kgateway/v2/pkg/a/a.go:14.1,16.2 3 4
github.com/kgateway-dev/kgateway/v2/pkg/b/b.go:1.1,3.2 1 0
`

const diff = `diff --git a/pkg/a/a.go b/pkg/a/a.go
--- a/pkg/a/a.go
+++ b/pkg/a/a.go
@@ -9,0 +10,3 @@ func x() {
@@ -20 +15 @@ func y() {
@@ -30,0 +40,2 @@ func z() {
diff --git a/pkg/a/a_test.go b/pkg/a/a_test.go
--- a/pkg/a/a_test.go
+++ b/pkg/a/a_test.go
@@ -1,0 +1,5 @@
diff --git a/pkg/b/b.go b/pkg/b/b.go
--- a/pkg/b/b.go
+++ b/pkg/b/b.go
@@ -1,0 +2 @@
`

func TestMergeSumsCountsAcrossProfiles(t *testing.T) {
	dir := t.TempDir()
	u, e := filepath.Join(dir, "u.out"), filepath.Join(dir, "e.out")
	out := filepath.Join(dir, "merged.out")
	mustWrite(t, u, unitProfile)
	mustWrite(t, e, e2eProfile)

	if err := run(u+","+e, out, "", "", "", defaultExclude, 0); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if !strings.Contains(string(got), "pkg/a/a.go:14.1,16.2 3 4") {
		t.Fatalf("expected summed block, got:\n%s", got)
	}
}

func TestPatchCoverage(t *testing.T) {
	p := profile{}
	dir := t.TempDir()
	u, e := filepath.Join(dir, "u.out"), filepath.Join(dir, "e.out")
	mustWrite(t, u, unitProfile)
	mustWrite(t, e, e2eProfile)
	for _, f := range []string{u, e} {
		if err := readProfile(f, p); err != nil {
			t.Fatal(err)
		}
	}

	res := patchCoverage(p, parseAddedLines(diff), regexp.MustCompile(defaultExclude))
	// a.go: lines 10-12 covered by unit, line 15 covered via e2e; 40-41 not in a
	// block. b.go line 2 sits in an uncovered block. a_test.go is excluded.
	if res.instrumented != 5 || res.covered != 4 {
		t.Fatalf("got %d/%d, want 4/5", res.covered, res.instrumented)
	}
	if got := ranges(res.files[1].uncovered); got != "2" {
		t.Fatalf("uncovered = %q", got)
	}
}

func TestMinPatchFails(t *testing.T) {
	dir := t.TempDir()
	u, d := filepath.Join(dir, "u.out"), filepath.Join(dir, "d.diff")
	mustWrite(t, u, unitProfile)
	mustWrite(t, d, diff)
	if err := run(u, "", "", d, "", defaultExclude, 99); err == nil {
		t.Fatal("expected failure below threshold")
	}
	if err := run(u, "", "", d, "", defaultExclude, 10); err != nil {
		t.Fatal(err)
	}
}

func TestRanges(t *testing.T) {
	if got := ranges([]int{1, 2, 3, 7, 9, 10}); got != "1-3, 7, 9-10" {
		t.Fatal(got)
	}
}

func mustWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

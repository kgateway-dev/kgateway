package helpers

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestClaimFailureDump(t *testing.T) {
	root := t.TempDir()

	var claimed atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if ClaimFailureDump(root, 3) {
				claimed.Add(1)
			}
		})
	}
	wg.Wait()

	if got := claimed.Load(); got != 3 {
		t.Fatalf("claimed %d dumps from 20 concurrent callers, want the cap of 3", got)
	}
	if ClaimFailureDump(root, 3) {
		t.Fatal("claimed a dump after the cap was reached")
	}
	if !ClaimFailureDump(root, 4) {
		t.Fatal("raising the cap should free a slot")
	}
	if !ClaimFailureDump(root, 0) {
		t.Fatal("a cap of 0 should always allow the dump")
	}
}

func TestMaxFailureDumps(t *testing.T) {
	cases := []struct {
		value string
		want  int
	}{
		{"", DefaultMaxFailureDumps},
		{"nope", DefaultMaxFailureDumps},
		{"3", 3},
		{"0", 0},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if tc.value != "" {
				t.Setenv(testutils.MaxFailureDumps, tc.value)
			}
			if got := MaxFailureDumps(); got != tc.want {
				t.Fatalf("MaxFailureDumps() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFailureDumpLocation(t *testing.T) {
	root := "/repo/_test/bug_report/kind"
	dir := root + "/TestKgateway/Foo"

	for _, k := range []string{"GITHUB_SERVER_URL", "GITHUB_REPOSITORY", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"} {
		t.Setenv(k, "")
	}
	if got := FailureDumpLocation(root, dir); got != dir {
		t.Fatalf("outside CI: got %q, want the local dir %q", got, dir)
	}

	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "kgateway-dev/kgateway")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	want := "https://github.com/kgateway-dev/kgateway/actions/runs/123/attempts/2#artifacts (bug-report-kind-* artifact, path TestKgateway/Foo)"
	if got := FailureDumpLocation(root, dir); got != want {
		t.Fatalf("in CI: got %q, want %q", got, want)
	}
}

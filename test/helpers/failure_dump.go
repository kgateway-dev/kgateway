package helpers

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// DefaultMaxFailureDumps is how many failure dumps one bug report directory
// holds when MAX_FAILURE_DUMPS is unset. A broken PR fails most tests, and
// dumping each of them costs minutes and gigabytes for no extra insight; a
// single flake still gets its full dump.
const DefaultMaxFailureDumps = 10

// dumpSlotsDir holds one empty file per dump claimed under a bug report root,
// grouped by dumpSlotsKey.
const dumpSlotsDir = ".dump-slots"

// dumpSlotsKey scopes the cap to one CI run attempt, which spans the test
// process and any reruns gotestsum starts. Outside CI the bug report directory
// outlives the run, so the cap is scoped to this process instead.
func dumpSlotsKey() string {
	if runID := os.Getenv("GITHUB_RUN_ID"); runID != "" {
		return fmt.Sprintf("run-%s-%s-%s", runID, os.Getenv("GITHUB_RUN_ATTEMPT"), os.Getenv("GITHUB_JOB"))
	}
	return fmt.Sprintf("pid-%d", os.Getpid())
}

// MaxFailureDumps returns the MAX_FAILURE_DUMPS cap, or DefaultMaxFailureDumps
// when it is unset or invalid. A value of 0 or less removes the cap.
func MaxFailureDumps() int {
	v, ok := os.LookupEnv(testutils.MaxFailureDumps)
	if !ok {
		return DefaultMaxFailureDumps
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Printf("ignoring invalid %s=%q; using %d\n", testutils.MaxFailureDumps, v, DefaultMaxFailureDumps)
		return DefaultMaxFailureDumps
	}
	return n
}

// ClaimFailureDump reserves one of limit dump slots under root and reports
// whether the caller may dump. Slots are claimed by exclusively creating a
// file, so the cap holds across parallel tests and across the separate test
// processes that reruns start within one CI run. A limit of 0 or less
// always allows the dump.
func ClaimFailureDump(root string, limit int) bool {
	if limit <= 0 {
		return true
	}
	dir := filepath.Join(root, dumpSlotsDir, dumpSlotsKey())
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		fmt.Printf("error creating dump slot directory, dumping anyway: %v\n", err)
		return true
	}
	for i := range limit {
		f, err := os.OpenFile(filepath.Join(dir, strconv.Itoa(i)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return true
		}
		if !errors.Is(err, fs.ErrExist) {
			fmt.Printf("error claiming dump slot, dumping anyway: %v\n", err)
			return true
		}
	}
	return false
}

// FailureDumpLocation describes where a dump written to dir can be found: in CI,
// a link to the run's artifacts and the path inside the uploaded bug report
// artifact (which is rooted at root); locally, just dir.
func FailureDumpLocation(root, dir string) string {
	server, repo, runID := os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")
	if server == "" || repo == "" || runID == "" {
		return dir
	}
	url := fmt.Sprintf("%s/%s/actions/runs/%s", server, repo, runID)
	if attempt := os.Getenv("GITHUB_RUN_ATTEMPT"); attempt != "" {
		url += "/attempts/" + attempt
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		rel = dir
	}
	return fmt.Sprintf("%s#artifacts (bug-report-%s-* artifact, path %s)", url, filepath.Base(root), rel)
}

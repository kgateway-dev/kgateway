// Command covreport stitches Go coverage profiles produced by different test
// runs (unit shards, e2e shards, ...) into one profile, and reports how well
// the lines added by a pull request are covered by that merged profile.
//
// Usage:
//
//	covreport -profiles a.out,b.out -merged-out merged.out \
//	    -base origin/main -summary summary.md -min-patch 0
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// modulePath is stripped from profile file names to get repo-relative paths.
const modulePath = "github.com/kgateway-dev/kgateway/v2/"

// defaultExclude matches files that are not expected to be covered by tests.
const defaultExclude = `(_test\.go|\.pb(\.\w+)?\.go|zz_generated[^/]*\.go)$`

type blockKey struct {
	file                                         string
	startLine, startCol, endLine, endCol, nStmts int
}

type profile map[blockKey]int

func main() {
	var (
		profiles  = flag.String("profiles", "", "comma separated coverage profiles to merge")
		mergedOut = flag.String("merged-out", "", "write the merged profile here")
		base      = flag.String("base", "", "git ref to diff against for patch coverage (skipped if empty)")
		diffFile  = flag.String("diff-file", "", "read a unified diff from this file instead of running git diff")
		summary   = flag.String("summary", "", "write a markdown summary here (e.g. $GITHUB_STEP_SUMMARY)")
		exclude   = flag.String("exclude", defaultExclude, "regexp of changed files ignored for patch coverage")
		minPatch  = flag.Float64("min-patch", 0, "fail if patch coverage is below this percentage")
	)
	flag.Parse()

	if err := run(*profiles, *mergedOut, *base, *diffFile, *summary, *exclude, *minPatch); err != nil {
		fmt.Fprintln(os.Stderr, "covreport:", err)
		os.Exit(1)
	}
}

func run(profiles, mergedOut, base, diffFile, summary, exclude string, minPatch float64) error {
	if profiles == "" {
		return fmt.Errorf("-profiles is required")
	}
	merged := profile{}
	for _, f := range strings.Split(profiles, ",") {
		if err := readProfile(strings.TrimSpace(f), merged); err != nil {
			return err
		}
	}
	if mergedOut != "" {
		if err := writeProfile(mergedOut, merged); err != nil {
			return err
		}
	}

	var md strings.Builder
	covered, total := totals(merged)
	fmt.Fprintf(&md, "## Test coverage\n\n**Total:** %.1f%% of statements (%d/%d)\n\n", pct(covered, total), covered, total)

	var patchPct = 100.0
	if base != "" || diffFile != "" {
		diff, err := readDiff(base, diffFile)
		if err != nil {
			return err
		}
		res := patchCoverage(merged, parseAddedLines(diff), regexp.MustCompile(exclude))
		res.writeMarkdown(&md)
		patchPct = pct(res.covered, res.instrumented)
		if res.instrumented == 0 {
			patchPct = 100
		}
	}

	fmt.Print(md.String())
	if summary != "" {
		f, err := os.OpenFile(summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(md.String()); err != nil {
			return err
		}
	}
	if patchPct < minPatch {
		return fmt.Errorf("patch coverage %.1f%% is below the required %.1f%%", patchPct, minPatch)
	}
	return nil
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

func readProfile(name string, into profile) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		k, count, err := parseLine(line)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		// Blocks are summed across profiles, matching `-covermode=count|atomic`
		// semantics. For `set` mode any non-zero sum still means "covered".
		into[k] += count
	}
	return sc.Err()
}

var lineRe = regexp.MustCompile(`^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$`)

func parseLine(line string) (blockKey, int, error) {
	m := lineRe.FindStringSubmatch(line)
	if m == nil {
		return blockKey{}, 0, fmt.Errorf("malformed profile line %q", line)
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	return blockKey{m[1], n(m[2]), n(m[3]), n(m[4]), n(m[5]), n(m[6])}, n(m[7]), nil
}

func writeProfile(name string, p profile) error {
	keys := make([]blockKey, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.file != b.file {
			return a.file < b.file
		}
		if a.startLine != b.startLine {
			return a.startLine < b.startLine
		}
		return a.startCol < b.startCol
	})
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	// Merged counts are sums, so use a mode that is valid for `go tool cover`.
	fmt.Fprintln(w, "mode: atomic")
	for _, k := range keys {
		fmt.Fprintf(w, "%s:%d.%d,%d.%d %d %d\n", k.file, k.startLine, k.startCol, k.endLine, k.endCol, k.nStmts, p[k])
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return f.Close()
}

func totals(p profile) (covered, total int) {
	for k, c := range p {
		total += k.nStmts
		if c > 0 {
			covered += k.nStmts
		}
	}
	return
}

func readDiff(base, diffFile string) (string, error) {
	if diffFile != "" {
		b, err := os.ReadFile(diffFile)
		return string(b), err
	}
	out, err := exec.Command("git", "diff", "-U0", "--no-color", "--no-ext-diff", base+"...HEAD", "--", "*.go").Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s...HEAD: %w", base, err)
	}
	return string(out), nil
}

var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// parseAddedLines returns, per repo-relative file, the new-side line numbers
// added or modified by a `git diff -U0` style diff.
func parseAddedLines(diff string) map[string][]int {
	added := map[string][]int{}
	var file string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = ""
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				file = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@") && file != "":
			m := hunkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			for i := 0; i < count; i++ {
				added[file] = append(added[file], start+i)
			}
		}
	}
	return added
}

type fileResult struct {
	name                  string
	covered, instrumented int
	uncovered             []int
}

type patchResult struct {
	covered, instrumented int
	files                 []fileResult
}

func patchCoverage(p profile, added map[string][]int, exclude *regexp.Regexp) patchResult {
	byFile := map[string][]blockKey{}
	for k := range p {
		name := strings.TrimPrefix(k.file, modulePath)
		byFile[name] = append(byFile[name], k)
	}

	var res patchResult
	names := make([]string, 0, len(added))
	for n := range added {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if path.Ext(name) != ".go" || exclude.MatchString(name) {
			continue
		}
		fr := fileResult{name: name}
		for _, line := range added[name] {
			inBlock, hit := false, false
			for _, b := range byFile[name] {
				if line >= b.startLine && line <= b.endLine {
					inBlock = true
					hit = hit || p[b] > 0
				}
			}
			if !inBlock { // comment, blank line, declaration: nothing to cover
				continue
			}
			fr.instrumented++
			if hit {
				fr.covered++
			} else {
				fr.uncovered = append(fr.uncovered, line)
			}
		}
		if fr.instrumented > 0 {
			res.files = append(res.files, fr)
			res.covered += fr.covered
			res.instrumented += fr.instrumented
		}
	}
	return res
}

func (r patchResult) writeMarkdown(w *strings.Builder) {
	if r.instrumented == 0 {
		w.WriteString("**Patch:** no testable Go lines were added or changed.\n")
		return
	}
	fmt.Fprintf(w, "**Patch:** %d of %d added/changed lines are covered by tests (%.1f%%)\n\n",
		r.covered, r.instrumented, pct(r.covered, r.instrumented))
	w.WriteString("| File | Covered | Uncovered lines |\n|---|---|---|\n")
	for _, f := range r.files {
		fmt.Fprintf(w, "| `%s` | %d/%d | %s |\n", f.name, f.covered, f.instrumented, ranges(f.uncovered))
	}
}

// ranges renders sorted line numbers as "3-5, 9".
func ranges(lines []int) string {
	if len(lines) == 0 {
		return "-"
	}
	var parts []string
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		if j == i {
			parts = append(parts, strconv.Itoa(lines[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", lines[i], lines[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

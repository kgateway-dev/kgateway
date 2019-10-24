package license

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/solo-io/go-list-licenses/assets"
)

type Template struct {
	Title    string
	Nickname string
	Words    map[string]int
}

func parseTemplate(content string) (*Template, error) {
	t := Template{}
	text := []byte{}
	state := 0
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if state == 0 {
			if line == "---" {
				state = 1
			}
		} else if state == 1 {
			if line == "---" {
				state = 2
			} else {
				if strings.HasPrefix(line, "title:") {
					t.Title = strings.TrimSpace(line[len("title:"):])
				} else if strings.HasPrefix(line, "nickname:") {
					t.Nickname = strings.TrimSpace(line[len("nickname:"):])
				}
			}
		} else if state == 2 {
			text = append(text, scanner.Bytes()...)
			text = append(text, []byte("\n")...)
		}
	}
	t.Words = makeWordSet(text)
	return &t, scanner.Err()
}

func loadTemplates() ([]*Template, error) {
	templates := []*Template{}
	for _, a := range assets.Assets {
		templ, err := parseTemplate(a.Content)
		if err != nil {
			return nil, err
		}
		templates = append(templates, templ)
	}
	return templates, nil
}

var (
	reWords     = regexp.MustCompile(`[\w']+`)
	reCopyright = regexp.MustCompile(
		`(?i)\s*Copyright (?:©|\(c\)|\xC2\xA9)?\s*(?:\d{4}|\[year\]).*`)
)

func cleanLicenseData(data []byte) []byte {
	data = bytes.ToLower(data)
	data = reCopyright.ReplaceAll(data, nil)
	return data
}

func makeWordSet(data []byte) map[string]int {
	words := map[string]int{}
	data = cleanLicenseData(data)
	matches := reWords.FindAll(data, -1)
	for i, m := range matches {
		s := string(m)
		if _, ok := words[s]; !ok {
			// Non-matching words are likely in the license header, to mention
			// copyrights and authors. Try to preserve the initial sequences,
			// to display them later.
			words[s] = i
		}
	}
	return words
}

type Word struct {
	Text string
	Pos  int
}

type sortedWords []Word

func (s sortedWords) Len() int {
	return len(s)
}

func (s sortedWords) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortedWords) Less(i, j int) bool {
	return s[i].Pos < s[j].Pos
}

type MatchResult struct {
	Template     *Template
	Score        float64
	ExtraWords   []string
	MissingWords []string
	FileContent  []byte
}

func sortAndReturnWords(words []Word) []string {
	sort.Sort(sortedWords(words))
	tokens := []string{}
	for _, w := range words {
		tokens = append(tokens, w.Text)
	}
	return tokens
}

// matchTemplates returns the best license template matching supplied data,
// its score between 0 and 1 and the list of words appearing in license but not
// in the matched template.
func matchTemplates(license []byte, templates []*Template) MatchResult {
	bestScore := float64(-1)
	var bestTemplate *Template
	bestExtra := []Word{}
	bestMissing := []Word{}
	words := makeWordSet(license)
	for _, t := range templates {
		extra := []Word{}
		missing := []Word{}
		common := 0
		for w, pos := range words {
			_, ok := t.Words[w]
			if ok {
				common++
			} else {
				extra = append(extra, Word{
					Text: w,
					Pos:  pos,
				})
			}
		}
		for w, pos := range t.Words {
			if _, ok := words[w]; !ok {
				missing = append(missing, Word{
					Text: w,
					Pos:  pos,
				})
			}
		}
		score := 2 * float64(common) / (float64(len(words)) + float64(len(t.Words)))
		if score > bestScore {
			bestScore = score
			bestTemplate = t
			bestMissing = missing
			bestExtra = extra
		}
	}
	return MatchResult{
		Template:     bestTemplate,
		Score:        bestScore,
		ExtraWords:   sortAndReturnWords(bestExtra),
		MissingWords: sortAndReturnWords(bestMissing),
		FileContent:  license,
	}
}

// fixEnv returns a copy of the process environment where GOPATH is adjusted to
// supplied value. It returns nil if gopath is empty.
func fixEnv(gopath string) []string {
	if gopath == "" {
		return nil
	}
	kept := []string{
		"GOPATH=" + gopath,
	}
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GOPATH=") {
			kept = append(kept, env)
		}
	}
	return kept
}

type MissingError struct {
	Err string
}

func (err *MissingError) Error() string {
	return err.Err
}

// expandPackages takes a list of package or package expressions and invoke go
// list to expand them to packages. In particular, it handles things like "..."
// and ".".
func expandPackages(gopath string, pkgs []string) ([]string, error) {
	args := []string{"list"}
	args = append(args, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Env = fixEnv(gopath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		if strings.Contains(output, "cannot find package") ||
			strings.Contains(output, "no buildable Go source files") {
			return nil, &MissingError{Err: output}
		}
		return nil, fmt.Errorf("'go %s' failed with:\n%s",
			strings.Join(args, " "), output)
	}
	names := []string{}
	for _, s := range strings.Split(string(out), "\n") {
		s = strings.TrimSpace(s)
		if s != "" {
			names = append(names, s)
		}
	}
	return names, nil
}

func listPackagesAndDeps(gopath string, pkgs []string) ([]string, error) {
	pkgs, err := expandPackages(gopath, pkgs)
	if err != nil {
		return nil, err
	}
	args := []string{"list", "-f", "{{range .Deps}}{{.}}|{{end}}"}
	args = append(args, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Env = fixEnv(gopath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		if strings.Contains(output, "cannot find package") ||
			strings.Contains(output, "no buildable Go source files") {
			return nil, &MissingError{Err: output}
		}
		return nil, fmt.Errorf("'go %s' failed with:\n%s",
			strings.Join(args, " "), output)
	}
	deps := []string{}
	seen := map[string]bool{}
	for _, s := range strings.Split(string(out), "|") {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			deps = append(deps, s)
			seen[s] = true
		}
	}
	for _, pkg := range pkgs {
		if !seen[pkg] {
			seen[pkg] = true
			deps = append(deps, pkg)
		}
	}
	sort.Strings(deps)
	return deps, nil
}

func listStandardPackages(gopath string) ([]string, error) {
	return expandPackages(gopath, []string{"std", "cmd"})
}

type PkgError struct {
	Err string
}

type PkgInfo struct {
	Name       string
	Dir        string
	Root       string
	ImportPath string
	Error      *PkgError
}

func getPackagesInfo(gopath string, pkgs []string) ([]*PkgInfo, error) {
	args := []string{"list", "-e", "-json"}
	// TODO: split the list for platforms which do not support massive argument
	// lists.
	args = append(args, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Env = fixEnv(gopath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go %s failed with:\n%s",
			strings.Join(args, " "), string(out))
	}
	infos := make([]*PkgInfo, 0, len(pkgs))
	decoder := json.NewDecoder(bytes.NewBuffer(out))
	for _, pkg := range pkgs {
		info := &PkgInfo{}
		err := decoder.Decode(info)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve package information for %s", pkg)
		}
		if pkg != info.ImportPath {
			return nil, fmt.Errorf("package information mismatch: asked for %s, got %s",
				pkg, info.ImportPath)
		}
		if info.Error != nil && info.Name == "" {
			info.Name = info.ImportPath
		}
		infos = append(infos, info)
	}
	return infos, err
}

var (
	reLicense = regexp.MustCompile(`(?i)^(?:` +
		`((?:un)?licen[sc]e)|` +
		`((?:un)?licen[sc]e\.(?:md|markdown|txt))|` +
		`(copy(?:ing|right)(?:\.[^.]+)?)|` +
		`(licen[sc]e\.[^.]+)` +
		`)$`)
)

// scoreLicenseName returns a factor between 0 and 1 weighting how likely
// supplied filename is a license file.
func scoreLicenseName(name string) float64 {
	m := reLicense.FindStringSubmatch(name)
	switch {
	case m == nil:
		break
	case m[1] != "":
		return 1.0
	case m[2] != "":
		return 0.9
	case m[3] != "":
		return 0.8
	case m[4] != "":
		return 0.7
	}
	return 0.
}

// findLicense looks for license files in package import path, and down to
// parent directories until a file is found or $GOPATH/src is reached. It
// returns the path and score of the best entry, an empty string if none was
// found.
func findLicense(info *PkgInfo) (string, error) {
	path := info.ImportPath
	for ; path != "."; path = filepath.Dir(path) {
		fis, err := ioutil.ReadDir(filepath.Join(info.Root, "src", path))
		if err != nil {
			return "", err
		}
		bestScore := float64(0)
		bestName := ""
		for _, fi := range fis {
			if !fi.Mode().IsRegular() {
				continue
			}
			score := scoreLicenseName(fi.Name())
			if score > bestScore {
				bestScore = score
				bestName = fi.Name()
			}
		}
		if bestName != "" {
			return filepath.Join(path, bestName), nil
		}
	}
	return "", nil
}

type License struct {
	Package  string
	Score    float64
	Template *Template
	Path     string
	// ManualPath indicates the URI of the license file. If provided, overrides the auto-generated URI path
	ManualPath   string
	Err          string
	ExtraWords   []string
	MissingWords []string
	// text from the license file
	FileContent []byte
}

func listLicenses(gopath string, pkgs []string) ([]License, error) {
	templates, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	deps, err := listPackagesAndDeps(gopath, pkgs)
	if err != nil {
		if _, ok := err.(*MissingError); ok {
			return nil, err
		}
		return nil, fmt.Errorf("could not list %s dependencies: %s",
			strings.Join(pkgs, " "), err)
	}
	std, err := listStandardPackages(gopath)
	if err != nil {
		return nil, fmt.Errorf("could not list standard packages: %s", err)
	}
	stdSet := map[string]bool{}
	for _, n := range std {
		stdSet[n] = true
	}
	infos, err := getPackagesInfo(gopath, deps)
	if err != nil {
		return nil, err
	}

	// Cache matched licenses by path. Useful for package with a lot of
	// subpackages like bleve.
	matched := map[string]MatchResult{}

	licenses := []License{}
	for _, info := range infos {
		if info.Error != nil {
			licenses = append(licenses, License{
				Package: info.Name,
				Err:     info.Error.Err,
			})
			continue
		}
		if stdSet[info.ImportPath] {
			continue
		}
		path, err := findLicense(info)
		if err != nil {
			return nil, err
		}
		license := License{
			Package: info.ImportPath,
			Path:    path,
		}
		if path != "" {
			fpath := filepath.Join(info.Root, "src", path)
			m, ok := matched[fpath]
			if !ok {
				data, err := ioutil.ReadFile(fpath)
				if err != nil {
					return nil, err
				}
				m = matchTemplates(data, templates)
				matched[fpath] = m
			}
			license.Score = m.Score
			license.Template = m.Template
			license.ExtraWords = m.ExtraWords
			license.MissingWords = m.MissingWords
			license.FileContent = m.FileContent
		}
		licenses = append(licenses, license)
	}
	return licenses, nil
}

// longestCommonPrefix returns the longest common prefix over import path
// components of supplied licenses.
func longestCommonPrefix(licenses []License) string {
	type Node struct {
		Name     string
		Children map[string]*Node
	}
	// Build a prefix tree. Not super efficient, but easy to do.
	root := &Node{
		Children: map[string]*Node{},
	}
	for _, l := range licenses {
		n := root
		for _, part := range strings.Split(l.Package, "/") {
			c := n.Children[part]
			if c == nil {
				c = &Node{
					Name:     part,
					Children: map[string]*Node{},
				}
				n.Children[part] = c
			}
			n = c
		}
	}
	n := root
	prefix := []string{}
	for {
		if len(n.Children) != 1 {
			break
		}
		for _, c := range n.Children {
			prefix = append(prefix, c.Name)
			n = c
			break
		}
	}
	return strings.Join(prefix, "/")
}

// groupLicenses returns the input licenses after grouping them by license path
// and find their longest import path common prefix. Entries with empty paths
// are left unchanged.
func groupLicenses(licenses []License) ([]License, error) {
	paths := map[string][]License{}
	for _, l := range licenses {
		if l.Path == "" {
			continue
		}
		paths[l.Path] = append(paths[l.Path], l)
	}
	for k, v := range paths {
		if len(v) <= 1 {
			continue
		}
		prefix := longestCommonPrefix(v)
		if prefix == "" {
			return nil, fmt.Errorf(
				"packages share the same license but not common prefix: %v", v)
		}
		l := v[0]
		l.Package = prefix
		paths[k] = []License{l}
	}
	kept := []License{}
	for _, l := range licenses {
		if l.Path == "" {
			kept = append(kept, l)
			continue
		}
		if v, ok := paths[l.Path]; ok {
			kept = append(kept, v[0])
			delete(paths, l.Path)
		}
	}
	return kept, nil
}

type Options struct {
	RunAll                  bool
	Words                   bool
	PrintConfidence         bool
	UseCsv                  bool
	PrunePath               string
	HelperListGlooPkgs      bool
	ConsolidatedLicenseFile string
	Pkgs                    []string
	Product                 Product
}

func PrintLicenses() error {
	opts := &Options{}
	flag.Usage = func() {
		fmt.Println(`Usage: licenses IMPORTPATH...

licenses lists all dependencies of specified packages or commands, excluding
standard library packages, and prints their licenses. Licenses are detected by
looking for files named like LICENSE, COPYING, COPYRIGHT and other variants in
the package directory, and its parent directories until one is found. Files
content is matched against a set of well-known licenses and the best match is
displayed along with its score.

With -a, all individual packages are displayed instead of grouping them by
license files.
With -w, words in package license file not found in the template license are
displayed. It helps assessing the changes importance.

Wrap PrintLicensesWithOptions with a go script if you would like to implement the Product interface.
The Product interface allows you to append or skip licenses.`)
		os.Exit(1)
	}
	flag.BoolVar(&opts.RunAll, "a", false, "display all individual packages")
	flag.BoolVar(&opts.Words, "w", false, "display words not matching license template")
	flag.BoolVar(&opts.PrintConfidence, "print-confidence", false, "display confidence level (default false)")
	flag.BoolVar(&opts.UseCsv, "csv", false, "print in csv format (default false)")
	flag.StringVar(&opts.PrunePath, "prune-path", "", "prefix path to remove from the package and file specs during display output, ex: 'github.com/solo-io/gloo/vendor/'")
	flag.BoolVar(&opts.HelperListGlooPkgs, "helper-list-gloo-pkgs", false, "if set, will just print the list of packages concerning Gloo")
	flag.StringVar(&opts.ConsolidatedLicenseFile, "consolidated-license-file", "", "if set, will write all of the licenses' text to this file")
	flag.Parse()
	opts.Pkgs = flag.Args()
	opts.Product = &genericProduct{}
	return PrintLicensesWithOptions(opts)

}
func PrintLicensesWithOptions(opts *Options) error {
	if opts.HelperListGlooPkgs {
		printGlooPkgNames()
		return nil
	}
	replacer := getPathReplacer(opts.Product.ReplacementList())
	if len(opts.Pkgs) < 1 {
		return fmt.Errorf("expect at least one package argument")
	}

	confidence := 0.9
	licenses, err := listLicenses("", opts.Pkgs)
	if err != nil {
		return err
	}
	if !opts.RunAll {
		licenses, err = groupLicenses(licenses)
		if err != nil {
			return err
		}
	}
	licenses = append(licenses, opts.Product.ExtraLicenses()...)
	w := tabwriter.NewWriter(os.Stdout, 1, 4, 2, ' ', 0)
	csvW := csv.NewWriter(os.Stdout)
	var includedLicenses []License
	for _, l := range licenses {
		if opts.Product.SkipLicense(l) {
			continue
		}
		license := "?"
		if l.Template != nil {
			if l.Score > .99 {
				license = fmt.Sprintf("%s", l.Template.Title)
				includedLicenses = append(includedLicenses, l)
			} else if l.Score >= confidence {
				includedLicenses = append(includedLicenses, l)
				if opts.PrintConfidence {
					license = fmt.Sprintf("%s (%2d%%)", l.Template.Title, int(100*l.Score))
				} else {
					license = fmt.Sprintf("%s", l.Template.Title)
				}
				if opts.Words && len(l.ExtraWords) > 0 {
					license += "\n\t+words: " + strings.Join(l.ExtraWords, ", ")
				}
				if opts.Words && len(l.MissingWords) > 0 {
					license += "\n\t-words: " + strings.Join(l.MissingWords, ", ")
				}
			} else {
				if opts.PrintConfidence {
					license = fmt.Sprintf("? (%s, %2d%%)", l.Template.Title, int(100*l.Score))
				} else {
					license = "UNKNOWN"
				}
			}
		} else if l.Err != "" {
			license = strings.Replace(l.Err, "\n", " ", -1)
		}
		packageString := l.Package
		pathString := l.ManualPath
		if pathString == "" {
			pathString = getPathString(l.Path, opts.PrunePath, replacer)
		}
		if opts.PrunePath != "" {
			packageString = strings.TrimPrefix(packageString, opts.PrunePath)
		}
		license = opts.Product.OverrideLicense(packageString, license)
		if opts.UseCsv {
			err = csvW.Write([]string{packageString, pathString, license})
		} else {
			_, err = w.Write([]byte(packageString + "\t" + license + "\n"))
		}
		if err != nil {
			return err
		}
	}
	if opts.ConsolidatedLicenseFile != "" {
		if err := writeConsolidatedLicenseFile(opts.ConsolidatedLicenseFile, includedLicenses); err != nil {
			return fmt.Errorf("unable to write consolidated license file %v", err)
		}
	}
	if opts.UseCsv {
		csvW.Flush()
		return nil
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}

func writeConsolidatedLicenseFile(outFile string, licenses []License) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	for i, l := range licenses {
		if _, err := f.WriteString(fmt.Sprintf("---\nIndex: %v\nPackage: %v\nLicense:\n", i, l.Package)); err != nil {
			return err
		}
		if _, err := f.Write(l.FileContent); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func getPathReplacer(oldnew []string) *strings.Replacer {
	return strings.NewReplacer(oldnew...)
}

// try to trim and substitute path elements, if available
func getPathString(rawPath, trimPrefix string, replacer *strings.Replacer) string {
	if trimPrefix == "" {
		return rawPath
	}
	pathString := strings.TrimPrefix(rawPath, trimPrefix)
	pathString = replacer.Replace(pathString)
	parts := strings.Split(pathString, "/")
	if len(parts) == 0 {
		return ""
	}
	switch {
	case len(parts) < 3:
		return pathString
	case parts[0] == "github.com":
		githubFilepathParts := []string{parts[0], parts[1], parts[2], "blob/master"}
		githubFilepathParts = append(githubFilepathParts, parts[3:]...)
		refinedString := strings.Join(githubFilepathParts, "/")
		return refinedString
	case parts[0] == "gopkg.in":
		// gopkg.in/pkg.v3      → github.com/go-pkg/pkg (branch/tag v3, v3.N, or v3.N.M)
		// gopkg.in/user/pkg.v3 → github.com/user/pkg   (branch/tag v3, v3.N, or v3.N.M)
		githubFilepathParts := []string{"github.com"}
		hasUserSpec := len(strings.Split(parts[1], ".")) == 1
		if hasUserSpec {
			userSpec := parts[1]
			pkgSpec := parts[2]
			pkgParts := strings.Split(pkgSpec, ".")
			if len(pkgParts) != 2 {
				return "UNKNOWN-version parse error"
			}
			version := pkgParts[1]
			githubFilepathParts = append(githubFilepathParts, userSpec, "blob", version)
			githubFilepathParts = append(githubFilepathParts, parts[3:]...)
			refinedString := strings.Join(githubFilepathParts, "/")
			return refinedString
		} else {
			pkgSpec := parts[1]
			pkgParts := strings.Split(pkgSpec, ".")
			if len(pkgParts) != 2 {
				return "UNKNOWN-version parse error"
			}
			pkgName := pkgParts[0]
			version := pkgParts[1]
			githubFilepathParts = append(githubFilepathParts, fmt.Sprintf("go-%v", pkgName), "blob", version)
			githubFilepathParts = append(githubFilepathParts, parts[2:]...)
			refinedString := strings.Join(githubFilepathParts, "/")
			return refinedString
		}
	}
	fmt.Printf("\n-----------\nDID NOT MATCH: %v\n--------\n", pathString)
	return pathString
}

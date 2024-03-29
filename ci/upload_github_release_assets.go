package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/pkgmgmtutils"
	"github.com/solo-io/go-utils/pkgmgmtutils/formula_updater_types"
	"github.com/solo-io/go-utils/versionutils"
)

func main() {
	ctx := context.Background()
	assetsOnly := false
	dryRun := false
	if len(os.Args) > 1 {
		var err error
		assetsOnly, err = strconv.ParseBool(os.Args[1])
		if err != nil {
			log.Fatalf("Unable to parse `assets_only` boolean argument, was provided %v", os.Args[1])
		}
	}
	if len(os.Args) > 2 {
		var err error
		dryRun, err = strconv.ParseBool(os.Args[2])
		if err != nil {
			log.Fatalf("Unable to parse `dry_run` boolean argument, was provided %v", os.Args[2])
		}
	}
	const buildDir = "_output"
	const repoOwner = "solo-io"
	const repoName = "gloo"

	versionBeingReleased := getReleaseVersionOrExitGracefully(dryRun)

	validateReleaseVersionOfCli(dryRun)

	assets := []githubutils.ReleaseAssetSpec{
		{
			Name:       "glooctl-linux-amd64",
			ParentPath: buildDir,
			UploadSHA:  true,
		},
		{
			Name:       "glooctl-linux-arm64",
			ParentPath: buildDir,
			UploadSHA:  true,
		},
		{
			Name:       "glooctl-darwin-amd64",
			ParentPath: buildDir,
			UploadSHA:  true,
		},
		{
			Name:       "glooctl-darwin-arm64",
			ParentPath: buildDir,
			UploadSHA:  true,
		},
		{
			Name:       "glooctl-windows-amd64.exe",
			ParentPath: buildDir,
			UploadSHA:  true,
		},
	}

	if dryRun {
		return
	}

	spec := githubutils.UploadReleaseAssetSpec{
		Owner:             repoOwner,
		Repo:              repoName,
		Assets:            assets,
		SkipAlreadyExists: true,
	}
	githubutils.UploadReleaseAssetCli(&spec)

	if assetsOnly {
		log.Warnf("Not creating PRs to update homebrew formulas or fish food because this was an assets_only release")
		return
	}
	mustUpdateFormulas(ctx, versionBeingReleased, repoOwner, repoName)
}

func mustUpdateFormulas(ctx context.Context, versionBeingReleased *versionutils.Version, repoOwner, repoName string) {
	fOpts := []*formula_updater_types.FormulaOptions{
		{
			Name:           "homebrew-tap/glooctl",
			FormulaName:    "glooctl",
			Path:           "Formula/glooctl.rb",
			RepoOwner:      repoOwner,      // Make change in this repo
			RepoName:       "homebrew-tap", // assumes this repo is forked from PRRepoOwner
			PRRepoOwner:    repoOwner,      // Make PR to this repo
			PRRepoName:     "homebrew-tap",
			PRBranch:       "main",
			PRDescription:  "",
			PRCommitName:   "Solo-io Bot",
			PRCommitEmail:  "bot@solo.io",
			VersionRegex:   `version\s*"([0-9.]+)"`,
			DarwinShaRegex: `url\s*".*-darwin.*\W*sha256\s*"(.*)"`,
			LinuxShaRegex:  `url\s*".*-linux.*\W*sha256\s*"(.*)"`,
		},
		{
			Name:            "fish-food/glooctl",
			FormulaName:     "glooctl",
			Path:            "Food/glooctl.lua",
			RepoOwner:       repoOwner,
			RepoName:        "fish-food",
			PRRepoOwner:     "fishworks",
			PRRepoName:      "fish-food",
			PRBranch:        "main",
			PRDescription:   "",
			PRCommitName:    "Solo-io Bot",
			PRCommitEmail:   "bot@solo.io",
			VersionRegex:    `version\s*=\s*"([0-9.]+)"`,
			DarwinShaRegex:  `os\s*=\s*"darwin",\W*.*\W*.*\W*.*\W*sha256\s*=\s*"(.*)",`,
			LinuxShaRegex:   `os\s*=\s*"linux",\W*.*\W*.*\W*.*\W*sha256\s*=\s*"(.*)",`,
			WindowsShaRegex: `os\s*=\s*"windows",\W*.*\W*.*\W*.*\W*sha256\s*=\s*"(.*)",`,
		},
		{
			Name:            "homebrew-core/glooctl",
			FormulaName:     "glooctl",
			Path:            "Formula/glooctl.rb",
			RepoOwner:       repoOwner,
			RepoName:        "homebrew-core",
			PRRepoOwner:     "homebrew",
			PRRepoName:      "homebrew-core",
			PRBranch:        "main",
			PRDescription:   "Created by Solo-io Bot",
			PRCommitName:    "Solo-io Bot",
			PRCommitEmail:   "bot@solo.io",
			VersionRegex:    `tag:\s*"v([0-9.]+)",`,
			VersionShaRegex: `revision:\s*"(.*)"`,
		},
	}

	formulaUpdater, err := pkgmgmtutils.NewFormulaUpdaterWithDefaults(ctx)
	if err != nil {
		log.Fatalf("Error constructing formula updater: %+v", err)
	}

	status, err := formulaUpdater.Update(ctx, versionBeingReleased, repoOwner, repoName, fOpts)
	if err != nil {
		log.Fatalf("Error trying to update package manager formulas. Error was: %s", err.Error())
	}
	for _, s := range status {
		if !s.Updated {
			if s.Err != nil {
				log.Fatalf("Error while trying to update formula %s. Error was: %s", s.Name, s.Err.Error())
			} else {
				log.Fatalf("Error while trying to update formula %s. Error was nil", s.Name) // Shouldn't happen; really bad if it does
			}
		}
		if s.Err != nil {
			if s.Err == pkgmgmtutils.ErrAlreadyUpdated {
				log.Warnf("Formula %s was updated externally, so no updates applied during this release", s.Name)
			} else {
				log.Fatalf("Error updating Formula %s. Error was: %s", s.Name, s.Err.Error())
			}
		}
	}
}

func validateReleaseVersionOfCli(dryRun bool) {
	releaseVersion := getReleaseVersionOrExitGracefully(dryRun).String()[1:]
	name := fmt.Sprintf("_output/glooctl-%s-amd64", runtime.GOOS)
	cmd := exec.Command(name, "version")
	bytes, err := cmd.Output()
	if err != nil {
		log.Fatalf("Error while trying to validate artifact version. Error was: %s", err.Error())
	}
	if !strings.Contains(string(bytes), `"client": `) {
		log.Fatalf("Unexpected version output for glooctl: %s", string(bytes))
	}
	clientVersionStr := strings.TrimPrefix(string(bytes), "Server: version undefined, could not find any version of gloo running\n\n")

	m := map[string]interface{}{}
	err = json.Unmarshal([]byte(clientVersionStr), &m)
	cl, ok := m["client"]
	if !ok {
		log.Fatalf("no client field")
	}
	clM, ok := cl.(map[string]interface{})
	if !ok {
		log.Fatalf("failed to cast client field")
	}
	clVer, ok := clM["version"]
	if !ok {
		log.Fatalf("no client.version field")
	}
	clVerStr, ok := clVer.(string)
	if !ok {
		log.Fatalf("failed to cast client.version field")
	}

	if dryRun {
		clVerStr, err = validVersionFromPrVersion(clVerStr)
		if err != nil {
			log.Fatalf(err.Error())
		}

	}

	if releaseVersion != clVerStr {
		log.Fatalf("Expected to release artifacts for version %s, glooctl binary reported version %s", releaseVersion, clVerStr)
	}
}

// stolen from "github.com/solo-io/go-utils/versionutils", but changed the hardcoding of "TAGGED_VERSION" to "VERSION"
func getReleaseVersionOrExitGracefully(dryRun bool) *versionutils.Version {
	versionStr, present := os.LookupEnv("VERSION")
	if !present || versionStr == "" {
		fmt.Printf("VERSION not found in environment.\n")
		os.Exit(1)
	}
	if dryRun {
		var err error
		versionStr, err = validVersionFromPrVersion(versionStr)
		if err != nil {
			log.Fatalf("unable to parse valid semver from PR version: %v", err)
		}
	}
	tag := "v" + versionStr

	version, err := versionutils.ParseVersion(tag)
	if err != nil {
		fmt.Printf("VERSION %s is not a valid semver version.\n", tag)
		os.Exit(1)
	}
	return version
}

// we have to force the version into semver in order to test this. We do this by chopping the PR designation off
// when running as dry_run
func validVersionFromPrVersion(ver string) (string, error) {
	expectedLen := 2
	if strings.Contains(ver, "beta") || strings.Contains(ver, "rc") {
		expectedLen = 3
	}
	splitVer := strings.Split(ver, "-")
	if len(splitVer) != expectedLen {
		return "", eris.New("invalid format for PR version; expected v<MAJOR>.<MINOR>.<PATCH>-(beta|rc)<N>-<PR> for beta or rc and v<MAJOR>.<MINOR>.<PATCH>-<PR> for LTS")
	}
	return strings.Join(splitVer[:expectedLen-1], "-"), nil

}

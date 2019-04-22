package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/logger"
	"github.com/solo-io/go-utils/pkgmgmtutils"
	"github.com/solo-io/go-utils/versionutils"
)

func main() {
	const buildDir = "_output"
	const repoOwner = "solo-io"
	const repoName = "gloo"

	validateReleaseVersionOfCli()

	assets := make([]githubutils.ReleaseAssetSpec, 6)
	assets[0] = githubutils.ReleaseAssetSpec{
		Name:       "glooctl-linux-amd64",
		ParentPath: buildDir,
		UploadSHA:  true,
	}
	assets[1] = githubutils.ReleaseAssetSpec{
		Name:       "glooctl-darwin-amd64",
		ParentPath: buildDir,
		UploadSHA:  true,
	}
	assets[2] = githubutils.ReleaseAssetSpec{
		Name:       "glooctl-windows-amd64.exe",
		ParentPath: buildDir,
		UploadSHA:  true,
	}
	assets[3] = githubutils.ReleaseAssetSpec{
		Name:       "gloo-gateway.yaml",
		ParentPath: "install",
	}
	assets[4] = githubutils.ReleaseAssetSpec{
		Name:       "gloo-ingress.yaml",
		ParentPath: "install",
	}
	assets[5] = githubutils.ReleaseAssetSpec{
		Name:       "gloo-knative.yaml",
		ParentPath: "install",
	}

	spec := githubutils.UploadReleaseAssetSpec{
		Owner:             repoOwner,
		Repo:              repoName,
		Assets:            assets,
		SkipAlreadyExists: true,
	}
	githubutils.UploadReleaseAssetCli(&spec)

	fOpts := []pkgmgmtutils.FormulaOptions{
		{
			Name:           "homebrew-tap/glooctl",
			FormulaName:    "glooctl",
			Path:           "Formula/glooctl.rb",
			RepoOwner:      repoOwner,      // Make change in this repo
			RepoName:       "homebrew-tap", // assumes this repo is forked from PRRepoOwner
			PRRepoOwner:    repoOwner,      // Make PR to this repo
			PRRepoName:     "homebrew-tap",
			PRBranch:       "master",
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
			PRBranch:        "master",
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
			PRBranch:        "master",
			PRDescription:   "",
			PRCommitName:    "Solo-io Bot",
			PRCommitEmail:   "bot@solo.io",
			VersionRegex:    `:tag\s*=>\s*"v([0-9.]+)",`,
			VersionShaRegex: `:revision\s*=>\s*"(.*)"`,
		},
	}

	// Update package manager install formulas
	status, err := pkgmgmtutils.UpdateFormulas(repoOwner, repoName, buildDir,
		`gloo-(darwin|linux|windows).*\.sha256`, fOpts)
	if err != nil {
		logger.Fatalf("Error trying to update package manager formulas. Error was: %s", err.Error())
	}
	for _, s := range status {
		if !s.Updated {
			if s.Err != nil {
				logger.Fatalf("Error while trying to update formula %s. Error was: %s", s.Name, s.Err.Error())
			} else {
				logger.Fatalf("Error while trying to update formula %s. Error was nil", s.Name) // Shouldn't happen; really bad if it does
			}
		}
		if s.Err != nil {
			if s.Err == pkgmgmtutils.ErrAlreadyUpdated {
				logger.Warnf("Formula %s was updated externally, so no updates applied during this release", s.Name)
			} else {
				logger.Fatalf("Error updating Formula %s. Error was: %s", s.Name, s.Err.Error())
			}
		}
	}
}

func validateReleaseVersionOfCli() {
	releaseVersion := versionutils.GetReleaseVersionOrExitGracefully().String()[1:]
	name := fmt.Sprintf("_output/glooctl-%s-amd64", runtime.GOOS)
	cmd := exec.Command(name, "--version")
	bytes, err := cmd.Output()
	if err != nil {
		logger.Fatalf("Error while trying to validate artifact version. Error was: %s", err.Error())
	}
	if !strings.HasSuffix(string(bytes), fmt.Sprintf("version %s\n", releaseVersion)) {
		logger.Fatalf("Unexpected version output for glooctl: %s", string(bytes))
	}
}

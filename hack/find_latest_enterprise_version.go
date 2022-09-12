package main

import (
	"context"
	"io/ioutil"
	"log"
	"math"
	"os"

	"github.com/ghodss/yaml"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/vfsutils"
)

type changelogConfig struct {
	ActiveVersionSubdirectory string `yaml:"activeVersionSubdirectory"`
}

func (c *changelogConfig) GetChangelogDirPath() string {
	yamlFile, err := ioutil.ReadFile("changelog/validation.yaml")
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	return "changelog/" + c.ActiveVersionSubdirectory
}

func main() {
	ctx := context.Background()
	var c changelogConfig
	changelogDirPath := c.GetChangelogDirPath()
	repoRootPath := "."
	owner := "solo-io"
	repo := "gloo"

	localVersion, err := getLargestLocalChangelogVersion(ctx, repoRootPath, owner, repo, changelogDirPath)
	if err != nil {
		log.Fatal(err)
	}
	// only version constraints we care about come from Gloo major/minor version
	maxGlooEVersion := &versionutils.Version{
		Major: localVersion.Major,
		Minor: localVersion.Minor,
		Patch: math.MaxInt32,
	}

	os.Mkdir("./_output", 0755)
	f, err := os.Create("./_output/gloo-enterprise-version")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	enterpriseVersion, err := version.GetLatestHelmChartVersionWithMaxVersion(version.EnterpriseHelmRepoIndex, version.GlooEE, true, maxGlooEVersion)
	if err != nil {
		log.Fatal(err)
	}
	f.WriteString(enterpriseVersion)
	f.Sync()
}

func getLargestLocalChangelogVersion(ctx context.Context, repoRootPath, owner, repo, changelogDirPath string) (*versionutils.Version, error) {
	mountedRepo, err := vfsutils.NewLocalMountedRepoForFs(repoRootPath, owner, repo)
	if err != nil {
		return nil, changelogutils.MountLocalDirectoryError(err)
	}
	files, err := mountedRepo.ListFiles(ctx, changelogDirPath)
	if err != nil {
		return nil, changelogutils.ReadChangelogDirError(err)
	}
	zero := versionutils.Zero()
	largestVersion := &zero
	for _, file := range files {
		if file.IsDir() {
			curVersion, err := versionutils.ParseVersion(file.Name())
			if err != nil {
				continue
			}
			if curVersion.MustIsGreaterThan(*largestVersion) {
				largestVersion = curVersion
			}
		}
	}

	if largestVersion == &zero {
		return nil, errors.Errorf("unable to find any versions at repo root %v with changelog dir %v", repoRootPath, changelogDirPath)
	}

	return largestVersion, nil
}

package main

import (
	"fmt"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/logger"
	"github.com/solo-io/go-utils/versionutils"
	"os/exec"
	"runtime"
	"strings"
)

func main() {
	validateReleaseVersionOfCli()

	assets := make([]githubutils.ReleaseAssetSpec, 6)
	assets[0] = githubutils.ReleaseAssetSpec{
		Name:       "glooctl-linux-amd64",
		ParentPath: "_output",
		UploadSHA:  true,
	}
	assets[1] = githubutils.ReleaseAssetSpec{
		Name:       "glooctl-darwin-amd64",
		ParentPath: "_output",
		UploadSHA:  true,
	}
	assets[2] = githubutils.ReleaseAssetSpec{
		Name:       "glooctl-windows-amd64.exe",
		ParentPath: "_output",
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
		Owner:             "solo-io",
		Repo:              "gloo",
		Assets:            assets,
		SkipAlreadyExists: true,
	}
	githubutils.UploadReleaseAssetCli(&spec)
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


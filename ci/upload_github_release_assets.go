package main

import "github.com/solo-io/go-utils/githubutils"

func main() {
	assets := make([]githubutils.ReleaseAssetSpec, 3)
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


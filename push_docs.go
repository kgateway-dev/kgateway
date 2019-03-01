package main

import (
	"fmt"
	"github.com/solo-io/go-utils/docsutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"os"
)

func main() {
	tag, present := os.LookupEnv("TAGGED_VERSION")
	if !present || tag == "" {
		fmt.Printf("TAGGED_VERSION not found in environment, skipping docs PR.\n", tag)
		os.Exit(0)
	}
	_, err := versionutils.ParseVersion(tag)
	if err != nil {
		fmt.Printf("TAGGED_VERSION %s is not a valid semver version, skipping docs PR.\n", tag)
		os.Exit(0)
	}
	// assumes changelog goes into solo-docs/gloo/docs/changelog/gloo-changelog
	spec := docsutils.DocsPRSpec{
		Owner: "solo-io",
		Repo: "gloo",
		Product: "gloo",
		Project: "gloo",
		Tag: tag,
		ApiPaths: []string {
			"docs/v1/github.com/solo-io/gloo",
			"docs/v1/github.com/solo-io/solo-kit",
			"docs/v1/gogoproto",
			"docs/v1/google",
		},
		CliPrefix: "",
		CliPath: "",
	}

	err = docsutils.CreateDocsPRFromSpec(&spec)
	if err != nil {
		log.Fatalf(err.Error())
	}
}
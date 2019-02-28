package main

import (
	"fmt"
	"github.com/solo-io/go-utils/docsutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"os"
)

func main() {
	tag, present := os.LookupEnv("TAGGED_VERSION")
	if !present {
		fmt.Printf("TAGGED_VERSION not found in environment, skipping docs PR.")
		os.Exit(0)
	}
	// assumes changelog goes into solo-docs/gloo/docs/changelog/gloo-changelog
	err := docsutils.CreateDocsPRSimple("solo-io", "gloo", tag,
		"docs/v1/github.com/solo-io/gloo",
		"docs/v1/github.com/solo-io/solo-kit",
		"docs/v1/gogoproto",
		"docs/v1/google")
	if err != nil {
		log.Fatalf(err.Error())
	}
}
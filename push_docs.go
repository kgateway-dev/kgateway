package main

import (
	"github.com/solo-io/go-utils/docsutils"
)

func main() {
	// tag set by utility
	spec := docsutils.DocsPRSpec{
		Owner: "solo-io",
		Repo: "gloo",
		Product: "gloo",
		Project: "gloo",
		ApiPaths: []string {
			"docs/v1/github.com/solo-io/gloo",
			"docs/v1/github.com/solo-io/solo-kit",
			"docs/v1/gogoproto",
			"docs/v1/google",
		},
	}
	docsutils.PushDocsCli(&spec)
}
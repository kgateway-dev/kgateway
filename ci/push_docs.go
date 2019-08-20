package main

import (
	"github.com/solo-io/go-utils/docsutils"
)

func main() {
	spec := docsutils.DocsPRSpec{
		Owner:           "solo-io",
		Repo:            "gloo",
		Product:         "gloo",
		ChangelogPrefix: "gloo",
		ApiPaths: []string{
			"docs/v1/github.com/solo-io/gloo",
			"docs/v1/github.com/solo-io/solo-kit",
			"docs/v1/gogoproto",
			"docs/v1/google",
		},
		Files: docsutils.Files{{
			From: "docs/helm-values.md",
			To:   "docs/installation/gateway/kubernetes/values.txt",
		}},
	}
	docsutils.PushDocsCli(&spec)
}

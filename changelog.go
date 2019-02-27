package main

import (
	"github.com/solo-io/go-utils/docsutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

func main() {
	err := docsutils.CreateDocsPR("solo-io", "gloo", "gloo",
		"docs/v1/github.com/solo-io/gloo",
		"docs/v1/github.com/solo-io/solo-kit",
		"docs/v1/gogoproto",
		"docs/v1/google")
	if err != nil {
		log.Fatalf(err.Error())
	}
}
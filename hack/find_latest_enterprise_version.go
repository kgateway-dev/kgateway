package main

import (
	"os"

	"github.com/solo-io/gloo/pkg/version"
)

func main() {
	os.Mkdir("./_output", 0644)
	f, err := os.Create("./_output/gloo-enterprise-version")
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	enterpriseTag, err := version.GetLatestEnterpriseVersion(false)
	if err != nil {
		os.Exit(1)
	}
	f.WriteString(enterpriseTag)
	f.Sync()
}

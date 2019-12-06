package main

import (
	"github.com/solo-io/gloo/pkg/version"
	"os"
)

func main() {
	os.Mkdir("./_output", 0644)
	f, err := os.Create("./_output/gloo-enterprise-version")
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	enterpriseTag, err := version.GetEnterpriseTag()
	if err != nil {
		os.Exit(1)
	}
	f.WriteString(enterpriseTag)
	f.Sync()
}
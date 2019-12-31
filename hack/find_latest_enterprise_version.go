package main

import (
	"log"
	"os"

	"github.com/solo-io/gloo/pkg/version"
)

func main() {
	os.Mkdir("./_output", 0755)
	f, err := os.Create("./_output/gloo-enterprise-version")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	enterpriseVersion, err := version.GetLatestEnterpriseVersion(false)
	if err != nil {
		log.Fatal(err)
	}
	f.WriteString(enterpriseVersion)
	f.Sync()
}

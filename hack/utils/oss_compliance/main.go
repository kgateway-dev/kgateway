package main

import (
	"log"

	"github.com/solo-io/go-list-licenses/pkg/license"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("unable to run oss compliance check: %v\n", err)
	}
}

func run() error {
	glooOptions := &license.Options{
		RunAll:                  false,
		Words:                   false,
		PrintConfidence:         false,
		UseCsv:                  true,
		PrunePath:               "github.com/solo-io/gloo/vendor/",
		HelperListGlooPkgs:      false,
		ConsolidatedLicenseFile: "third_party_licenses.txt",
		ProductName:             "gloo",
		Pkgs: []string{
			"github.com/solo-io/gloo/projects/accesslogger/cmd",
			"github.com/solo-io/gloo/projects/discovery/cmd",
			"github.com/solo-io/gloo/projects/envoyinit/cmd",
			"github.com/solo-io/gloo/projects/gateway/cmd",
			"github.com/solo-io/gloo/projects/gloo/cmd",
			"github.com/solo-io/gloo/projects/ingress/cmd",
			"github.com/solo-io/gloo/projects/hypergloo",
		},
	}
	return license.PrintLicensesWithOptions(glooOptions)
}

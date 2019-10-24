package license

import (
	"fmt"
	"strings"
)

// since Gloo is the primary user of this library a few convenience functions are provided here
// (consider moving them to Gloo in the future)

var glooCommonBinaryPath = "github.com/solo-io/gloo/projects/%v/cmd"

var glooBinaries = []string{
	"accesslogger",
	"discovery",
	"envoyinit",
	"gateway",
	"gloo",
	"ingress",
	// non-standard projects:
	//"hypergloo",
	//"knative",
	//"clusteringress",
	//"metrics",
}
var glooBinariesNonStandard = []string{
	"github.com/solo-io/gloo/projects/hypergloo",
}

func printGlooPkgNames() {
	names := make([]string, len(glooBinaries)+len(glooBinariesNonStandard))
	var index int
	for i, b := range glooBinaries {
		names[i] = fmt.Sprintf(glooCommonBinaryPath, b)
		index = i
	}
	for _, b := range glooBinariesNonStandard {
		index++
		names[index] = b
	}
	// print in the form expected by the main license check - you can paste this result into the arguments list to
	// run the license analysis on all the binaries that Gloo produces
	fmt.Println(strings.Join(names, " "))
}

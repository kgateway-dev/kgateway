package printers_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/onsi/ginkgo"

	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestPrinters(t *testing.T) {
	//skhelpers.RegisterCommonFailHandlers() // these are currently overwritten by the fail handler below
	skhelpers.SetupLog()
	RegisterFailHandler(
		func(message string, callerSkip ...int) {
			fmt.Println("Fail handler msg", message)

			printTrimmedStack()

			Fail(message, callerSkip...)
		})
	RunSpecs(t, "Printer Suite")
}

// TODO(mitchdraft) - move this to go-utils
func printTrimmedStack() {
	stack := debug.Stack()
	fmt.Println(trimVendorStack(stack))
}

// SAMPLE OUTPUT OF debug.Stack()
// note the header line, followed by line pairs of the format "function_details\nfile_details"
/*
goroutine 38 [running]:
runtime/debug.Stack(0x1f39ae9, 0x1a, 0xc000134e70)
	/usr/local/Cellar/go/1.11.1/libexec/src/runtime/debug/stack.go:24 +0xa7
github.com/solo-io/gloo/projects/gloo/cli/pkg/printers_test.TestPrinters.func1(0xc000174c60, 0x114, 0xc000048928, 0x1, 0x1)
	/Users/mitch/go/src/github.com/solo-io/gloo/projects/gloo/cli/pkg/printers/printers_suite_test.go:27 +0x293
github.com/solo-io/gloo/vendor/github.com/onsi/gomega/internal/assertion.(*Assertion).match(0xc00040c8c0, 0x209f340, 0xc00058a5d0, 0x1, 0x0, 0x0, 0x0, 0xc00058a5d0)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/gomega/internal/assertion/assertion.go:75 +0x20e
github.com/solo-io/gloo/vendor/github.com/onsi/gomega/internal/assertion.(*Assertion).To(0xc00040c8c0, 0x209f340, 0xc00058a5d0, 0x0, 0x0, 0x0, 0xc000140910)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/gomega/internal/assertion/assertion.go:38 +0xca
github.com/solo-io/gloo/projects/gloo/cli/pkg/printers.glob..func1.5()
	/Users/mitch/go/src/github.com/solo-io/gloo/projects/gloo/cli/pkg/printers/virtualservice_test.go:166 +0xc6c
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/leafnodes.(*runner).runSync(0xc0004686c0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/leafnodes/runner.go:113 +0x9c
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/leafnodes.(*runner).run(0xc0004686c0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/leafnodes/runner.go:64 +0x12c
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/leafnodes.(*ItNode).Run(0xc000590920, 0x208e3c0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/leafnodes/it_node.go:26 +0x7f
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/spec.(*Spec).runSample(0xc00012e3c0, 0x0, 0x208e3c0, 0xc00058c840)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/spec/spec.go:215 +0x636
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/spec.(*Spec).Run(0xc00012e3c0, 0x208e3c0, 0xc00058c840)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/spec/spec.go:138 +0xf7
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/specrunner.(*SpecRunner).runSpec(0xc0000eac80, 0xc00012e3c0, 0x4)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/specrunner/spec_runner.go:200 +0x10f
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/specrunner.(*SpecRunner).runSpecs(0xc0000eac80, 0x1)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/specrunner/spec_runner.go:170 +0x328
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/specrunner.(*SpecRunner).Run(0xc0000eac80, 0xd)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/specrunner/spec_runner.go:66 +0x11f
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/suite.(*Suite).Run(0xc0004a21e0, 0x3c400a8, 0xc0000f0d00, 0x1f2a5ab, 0xd, 0xc000124590, 0x1, 0x1, 0x20af700, 0xc00058c840, ...)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/internal/suite/suite.go:62 +0x28f
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo.RunSpecsWithCustomReporters(0x208ee60, 0xc0000f0d00, 0x1f2a5ab, 0xd, 0xc00012df38, 0x1, 0x1, 0x1e08240)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/ginkgo_dsl.go:221 +0x247
github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo.RunSpecs(0x208ee60, 0xc0000f0d00, 0x1f2a5ab, 0xd, 0x1)
	/Users/mitch/go/src/github.com/solo-io/gloo/vendor/github.com/onsi/ginkgo/ginkgo_dsl.go:202 +0x89
github.com/solo-io/gloo/projects/gloo/cli/pkg/printers_test.TestPrinters(0xc0000f0d00)
	/Users/mitch/go/src/github.com/solo-io/gloo/projects/gloo/cli/pkg/printers/printers_suite_test.go:32 +0x146
testing.tRunner(0xc0000f0d00, 0x1fb2718)
	/usr/local/Cellar/go/1.11.1/libexec/src/testing/testing.go:827 +0xbf
created by testing.(*T).Run
	/usr/local/Cellar/go/1.11.1/libexec/src/testing/testing.go:878 +0x353
*/
func trimVendorStack(stack []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(stack))
	ind := -1
	pair := []string{}
	skipCount := 0
	output := ""
	for scanner.Scan() {
		ind++
		if ind == 0 {
			// skip the header
			continue
		}
		pair = append(pair, scanner.Text())
		if len(pair) == 2 {
			evaluateStackPair(pair[0], pair[1], &output, &skipCount)
			pair = []string{}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
	output = fmt.Sprintf("Stack trace (skipped %v entries that matched filter criteria):\n%v", skipCount, output)
	return output
}

var (
	funcRuntimeDebugRegex = &regexp.Regexp{}
	fileVendorRegex       = &regexp.Regexp{}
	fileSuiteRegex        = &regexp.Regexp{}
	fileGoTestLibRegex    = &regexp.Regexp{}
)

func init() {
	funcRuntimeDebugRegex = regexp.MustCompile("runtime/debug")
	fileVendorRegex = regexp.MustCompile("vendor")
	fileSuiteRegex = regexp.MustCompile("suite_test.go")
	fileGoTestLibRegex = regexp.MustCompile("src/testing/testing.go")
}

func evaluateStackPair(functionLine, fileLine string, output *string, skipCount *int) {
	skip := false
	if funcRuntimeDebugRegex.MatchString(functionLine) {
		skip = true
	}
	if fileVendorRegex.MatchString(fileLine) ||
		fileSuiteRegex.MatchString(fileLine) ||
		fileGoTestLibRegex.MatchString(fileLine) {
		skip = true
	}
	if skip {
		*skipCount = *skipCount + 1
		return
	}
	*output = fmt.Sprintf("%v%v\n%v\n", *output, functionLine, fileLine)
	return
}

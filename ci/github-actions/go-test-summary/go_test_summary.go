package main

import (
	"flag"

	"github.com/solo-io/gloo/ci/github-actions/go-test-summary/summary"
)

func main() {
	var logFile, outFile string
	var jsonOutput bool

	flag.StringVar(&logFile, "log-file", "./_test/test_log/go-test", "point to the raw string log output of go test command")
	flag.StringVar(&outFile, "out-file", "./_test/test_log/go-test-summary", "where to place the output summary file")
	flag.BoolVar(&jsonOutput, "json", false, "output as json")
	flag.Parse()

	summary.Main(logFile, outFile, jsonOutput)
}

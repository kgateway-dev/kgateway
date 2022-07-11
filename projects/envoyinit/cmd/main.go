package main

import (
	_ "github.com/solo-io/gloo/projects/envoyinit/hack/filter_types"
	"github.com/solo-io/gloo/projects/envoyinit/pkg/runner"
)

func main() {
	envoyExecutable := runner.GetEnvoyExecutable()
	inputPath := runner.GetInputConfigPath()
	outputPath := runner.GetOutputConfigPath()

	runner.RunEnvoy(envoyExecutable, inputPath, outputPath)
}

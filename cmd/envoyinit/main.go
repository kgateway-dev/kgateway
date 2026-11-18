package main

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/envoyinit"
)

func main() {
	envoyExecutable := envoyinit.GetEnvoyExecutable()
	inputPath := envoyinit.GetInputConfigPath()
	outputPath := envoyinit.GetOutputConfigPath()

	envoyinit.RunEnvoy(envoyExecutable, inputPath, outputPath)
}

package runner

import (
	"bytes"
	"log"
	"os"
	"syscall"

	"github.com/solo-io/gloo/projects/envoyinit/pkg/downward"
)

const (
	// Environment variable for the file that is used to inject input configuration used to bootstrap envoy
	inputConfigPathEnv     = "INPUT_CONF"
	defaultInputConfigPath = "/etc/envoy/envoy.yaml"

	// Environment variable for the file that is written to with transformed bootstrap configuration
	outputConfigPathEnv     = "OUTPUT_CONF"
	defaultOutputConfigPath = "/tmp/envoy.yaml"

	// Environment variable for the path to the envoy executable
	envoyExecutableEnv     = "ENVOY"
	defaultEnvoyExecutable = "/usr/local/bin/envoy"
)

// RunEnvoy run Envoy with bootstrap configuration injected from a file
func RunEnvoy(envoyExecutable, inputPath, outputPath string) {
	// 1. Transform the configuration using the Kubernetes Downward API
	bootstrapConfig, err := getAndTransformConfig(inputPath)
	if err != nil {
		log.Fatalf("initializer failed: %v", err)
	}

	// 2. Write to a file for debug purposes
	// since this operation is meant only for debug purposes, we ignore the error
	// this might fail if root fs is read only
	_ = os.WriteFile(outputPath, []byte(bootstrapConfig), 0444)

	// 3. Execute Envoy with the provided configuration
	args := []string{envoyExecutable, "--config-yaml", bootstrapConfig}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}
	if err = syscall.Exec(args[0], args, os.Environ()); err != nil {
		panic(err)
	}
}

// GetInputConfigPath returns the path to a file containing the Envoy bootstrap configuration
// This configuration may leverage the Kubernetes Downward API
// https://kubernetes.io/docs/tasks/inject-data-application/downward-api-volume-expose-pod-information/#the-downward-api
func GetInputConfigPath() string {
	return getEnvOrDefault(inputConfigPathEnv, defaultInputConfigPath)
}

// GetOutputConfigPath returns the path to a file where the raw Envoy bootstrap configuration will
// be persisted for debugging purposes
func GetOutputConfigPath() string {
	return getEnvOrDefault(outputConfigPathEnv, defaultOutputConfigPath)
}

// GetEnvoyExecutable returns the Envoy executable
func GetEnvoyExecutable() string {
	return getEnvOrDefault(envoyExecutableEnv, defaultEnvoyExecutable)
}

// getEnvOrDefault returns the value of the environment variable, if one exists, or a default string
func getEnvOrDefault(envName, defaultValue string) string {
	maybeEnvValue := os.Getenv(envName)
	if maybeEnvValue != "" {
		return maybeEnvValue
	}
	return defaultValue
}

// getAndTransformConfig reads a file, transforms it using the Downward API
// and returns the transformed configuration
func getAndTransformConfig(inputFile string) (string, error) {
	inReader, err := os.Open(inputFile)
	if err != nil {
		return "", err
	}
	defer inReader.Close()

	var buffer bytes.Buffer
	err = downward.Transform(inReader, &buffer)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

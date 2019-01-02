package main

import (
	"log"
	"os"
	"syscall"

	"github.com/solo-io/envoy-operator/pkg/downward"
)

func main() {
	inputfile := inputCfg()
	outfile := outputCfg()

	transformer := downward.NewTransformer()
	err := transformer.TransformFiles(inputfile, outfile)
	if err != nil {
		log.Fatalf("initializer failed: %v", err)
	}
	env := os.Environ()
	args := []string{envoy(), "-c", outfile, "--v2-config-only"}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}
	if err := syscall.Exec(args[0], args, env); err != nil {
		panic(err)
	}

}

func envoy() string {
	maybeEnvoy := os.Getenv("ENVOY")
	if maybeEnvoy != "" {
		return maybeEnvoy
	}
	return "/usr/local/bin/envoy"
}

func inputCfg() string {
	maybeConf := os.Getenv("INPUT_CONF")
	if maybeConf != "" {
		return maybeConf
	}
	return "/etc/envoy/envoy.yaml"
}

func outputCfg() string {
	maybeConf := os.Getenv("OUTPUT_CONF")
	if maybeConf != "" {
		return maybeConf
	}
	return "/tmp/envoy.yaml"
}

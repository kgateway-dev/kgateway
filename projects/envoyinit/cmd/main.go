package main

import (
	"io/ioutil"
	"log"
	"os"
	"syscall"

	"github.com/solo-io/gloo/projects/envoyinit/cmd/utils"
)

func writeConfig(cfg string) {
	ioutil.WriteFile(outputCfg(), []byte(cfg), 0444)
}

func main() {
	inputFile := inputCfg()
	outCfg, err := utils.GetConfig(inputFile)
	if err != nil {
		log.Fatalf("initializer failed: %v", err)
	}

	// best effort - write to a file for debug purposes.
	// this might fail if root fs is read only
	writeConfig(outCfg)

	env := os.Environ()
	args := []string{envoy(), "--config-yaml", outCfg}
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

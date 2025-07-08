package main

import (
	"os"

	"github.com/kgateway-dev/kgateway/v2/pkg/envoy"
)

func main() {
	os.Stdout.WriteString(envoy.Image)
	os.Exit(0)
}

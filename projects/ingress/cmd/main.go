package main

import (
	"github.com/solo-io/gloo/projects/ingress/pkg/runner"
	"github.com/solo-io/go-utils/log"
)

func main() {
	if err := runner.Run(nil); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

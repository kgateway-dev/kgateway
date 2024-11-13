package main

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/probes"
	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/go-utils/log"
)

func main() {
	ctx := context.Background()

	// Start a server which is responsible for responding to liveness probes
	probes.StartLivenessProbeServer(ctx)

	if err := setup.Main(ctx); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

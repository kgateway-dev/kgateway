package main

import (
	"context"

	"github.com/kgateway-dev/kgateway/pkg/utils/probes"
	"github.com/kgateway-dev/kgateway/projects/gateway2/setup"
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

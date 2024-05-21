package main

import (
	"context"
	"github.com/solo-io/gloo/projects/gloo/pkg/debug/inputsnapshot"
	"net/http"

	"github.com/solo-io/gloo/pkg/utils/probes"
	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

func main() {
	ctx := context.Background()

	// Start a server which is responsible for responding to liveness probes
	probes.StartLivenessProbeServer(ctx)

	// Start a server which is responsible for responding to debug requests
	startGenericDebugServer()

	if err := setup.Main(ctx); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

// startGenericDebugServer starts a server which handles requests from users of the product
// as they debug certain behaviors of the Gloo service.
// This is becoming the foundation for a generic Admin interface: https://github.com/solo-io/gloo/issues/6494
// There is _another_ server that has an overlapping responsibility, which can be configured
// by enabling dev_mode: https://github.com/solo-io/gloo/blob/fd331e502a7513376ac15bafb3621de70af98efb/projects/gloo/api/v1/settings.proto#L91
// Ideally we consolidate that API into this one as it is not widely known about or adopted
func startGenericDebugServer() {
	customDebugHandler := func(mux *http.ServeMux, profiles map[string]string) {

		mux.Handle("/input/snapshots", inputsnapshot.DefaultServer)
	}

	stats.ConditionallyStartStatsServer(customDebugHandler)
}

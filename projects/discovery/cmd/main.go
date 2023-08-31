package main

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	fdssetup "github.com/solo-io/gloo/projects/discovery/pkg/fds/setup"
	uds "github.com/solo-io/gloo/projects/discovery/pkg/uds/setup"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

const (
	discoveryComponentName = "discovery"
)

func main() {
	ctx := context.Background()
	setuputils.SetupLogging(ctx, discoveryComponentName)

	stats.ConditionallyStartStatsServer()
	if err := run(); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

func run() error {
	errs := make(chan error)
	go func() {
		errs <- uds.Main(nil)
	}()
	go func() {
		errs <- fdssetup.Main(nil)
	}()
	return <-errs
}

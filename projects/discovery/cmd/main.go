package main

import (
	"context"

	fdssetup "github.com/solo-io/gloo/projects/discovery/pkg/fds/setup"
	udssetup "github.com/solo-io/gloo/projects/discovery/pkg/uds/setup"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

func main() {
	stats.ConditionallyStartStatsServer()
	if err := run(); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

func run() error {
	errs := make(chan error)
	ctx := context.Background()

	go func() {
		errs <- fdssetup.Main(ctx)
	}()
	go func() {
		errs <- udssetup.Main(ctx)
	}()

	return <-errs
}

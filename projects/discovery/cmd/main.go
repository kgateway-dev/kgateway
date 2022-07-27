package main

import (
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"

	fdssetup "github.com/solo-io/gloo/projects/discovery/pkg/fds/runner"
	uds "github.com/solo-io/gloo/projects/discovery/pkg/uds/runner"
)

func main() {
	stats.ConditionallyStartStatsServer()
	if err := run(); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

func run() error {
	errs := make(chan error)
	go func() {
		errs <- uds.Run(nil)
	}()
	go func() {
		errs <- fdssetup.Run(nil)
	}()
	return <-errs
}

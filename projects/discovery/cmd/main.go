package main

import (
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/setup"
	setup2 "github.com/solo-io/gloo/projects/discovery/pkg/uds/setup"
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
	go func() {
		errs <- setup2.Main(nil)
	}()
	go func() {
		errs <- setup.Main(nil)
	}()
	return <-errs
}

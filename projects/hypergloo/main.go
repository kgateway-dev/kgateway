package main

import (
	"context"
	"flag"

	fdssetup "github.com/solo-io/gloo/projects/discovery/pkg/fds/setup"
	uds "github.com/solo-io/gloo/projects/discovery/pkg/uds/setup"
	gatewaysetup "github.com/solo-io/gloo/projects/gateway/pkg/setup"
	gloosetup "github.com/solo-io/gloo/projects/gloo/pkg/setup"
	ingresssetup "github.com/solo-io/gloo/projects/ingress/pkg/setup"
	"github.com/solo-io/go-utils/contextutils"
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
	contextutils.LoggerFrom(context.TODO()).Infof("hypergloo!")
	flag.Parse()
	errs := make(chan error)
	go func() {
		errs <- gloosetup.Main(nil)
	}()
	go func() {
		errs <- gatewaysetup.Main(nil)
	}()
	go func() {
		errs <- ingresssetup.Main(nil)
	}()
	go func() {
		errs <- uds.Main(nil)
	}()
	go func() {
		errs <- fdssetup.Main(nil)
	}()
	return <-errs
}

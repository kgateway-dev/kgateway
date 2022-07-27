package main

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

func main() {
	stats.ConditionallyStartStatsServer()

	if err := runner.Run(context.Background()); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

package main

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/setup"

	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

func main() {
	stats.ConditionallyStartStatsServer()

	if err := setup.Main(context.Background()); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

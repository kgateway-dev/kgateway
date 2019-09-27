package main

import (
	"context"
	"os"

	"github.com/solo-io/gloo/projects/gloo/pkg/setup"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

const (
	START_STATS_SERVER = "START_STATS_SERVER"
)

func main() {
	if os.Getenv(START_STATS_SERVER) != "" {
		stats.StartStatsServer()
	}

	if err := setup.Main(context.Background()); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

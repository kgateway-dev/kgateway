package main

import (
	"os"

	"github.com/solo-io/gloo/projects/metrics/pkg/runner"
	"github.com/solo-io/go-utils/stats"
)

const (
	START_STATS_SERVER = "START_STATS_SERVER"
)

func main() {
	if os.Getenv(START_STATS_SERVER) != "" {
		stats.StartStatsServer()
	}
	runner.Run()
}

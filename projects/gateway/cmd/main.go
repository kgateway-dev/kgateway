package main

import (
	"os"

	"github.com/solo-io/gloo/projects/gateway/pkg/setup"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"github.com/solo-io/solo-kit/pkg/utils/stats"
)

const (
	START_STATS_SERVER = "START_STATS_SERVER"
)

func main() {
	if os.Getenv(START_STATS_SERVER) != "" {
		stats.StartStatsServer()
	}
	if err := setup.Main(); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

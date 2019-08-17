package main

import (
	"os"

	"github.com/solo-io/gloo/projects/gateway/pkg/setup"
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
	if err := setup.Main(nil); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

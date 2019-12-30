package main

import (
	"github.com/solo-io/gloo/projects/accesslogger/pkg/runner"
	"github.com/solo-io/go-utils/stats"
)

func main() {
	stats.ConditionallyStartStatsServer()
	runner.Run()
}

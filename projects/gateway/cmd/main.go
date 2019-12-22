package main

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/setup"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/stats"
)

func main() {
	stats.ConditionallyStartStatsServer()
	if err := setup.Main(nil); err != nil {
		log.Fatalf("err in main: %v", err.Error())
	}
}

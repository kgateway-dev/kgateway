//go:build e2e

package helloworld_sigs

import (
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var testenv env.Environment

// TestMain initializes the test environment for the POC tests.
// This runs before any tests and sets up the base configuration.
func TestMain(m *testing.M) {
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		panic(err)
	}

	testenv = env.NewWithConfig(cfg)
	os.Exit(testenv.Run(m))
}

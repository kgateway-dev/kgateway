//go:build e2e

package basicrouting_sigs

import (
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

var testenv env.Environment

// TestMain initializes the test environment for sigs/e2e-framework tests.
// This runs before any tests and sets up the base configuration.
func TestMain(m *testing.M) {
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		panic(err)
	}

	testenv = env.NewWithConfig(cfg)

	if err := schemes.AddToScheme(cfg.Client().Resources().GetScheme()); err != nil {
		panic(err)
	}

	os.Exit(testenv.Run(m))
}

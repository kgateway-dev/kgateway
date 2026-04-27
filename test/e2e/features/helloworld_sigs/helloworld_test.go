//go:build e2e

package helloworld_sigs

import (
	"context"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// TestHelloWorld is a simple POC test that validates basic functionality using sigs/e2e-framework.
func TestHelloWorld(t *testing.T) {
	feat := features.New("Hello World POC").
		Assess("true is true", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			if true != true {
				t.Error("unexpected: true does not equal true")
			}
			return ctx
		}).
		Feature()

	testenv.Test(t, feat)
}

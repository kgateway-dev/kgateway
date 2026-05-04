package basicrouting_sigs

import (
	"os"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	// Register Gateway API types with the global scheme
	// The framework's client uses the global scheme by default
	gwv1.Install(scheme.Scheme)
	gwv1b1.Install(scheme.Scheme)

	cfg, err := envconf.NewFromFlags()
	if err != nil {
		panic(err)
	}

	testenv = env.NewWithConfig(cfg)
	os.Exit(testenv.Run(m))
}

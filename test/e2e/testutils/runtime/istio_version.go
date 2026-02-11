//go:build e2e

package runtime

import (
	"os"
	"testing"

	"github.com/Masterminds/semver/v3"
)

const (
	IstioVersionEnv     = "ISTIO_VERSION"
	DefaultIstioVersion = "1.25.1"
)

// ShouldSkipIstioVersion reports whether the current Istio version (from ISTIO_VERSION env)
// is older than minVersion. Use this to skip tests that require a minimum Istio version.
// It calls t.Fatalf if ISTIO_VERSION is unset or not valid semver.
func ShouldSkipIstioVersion(t *testing.T, minVersion string) bool {
	t.Helper()
	istioVersion, ok := os.LookupEnv(IstioVersionEnv)
	if !ok {
		t.Fatalf("required environment variable %s not set", IstioVersionEnv)
	}
	current, err := semver.NewVersion(istioVersion)
	if err != nil {
		t.Fatalf("failed to parse istio version %s as semver: %v", istioVersion, err)
	}
	return current.LessThan(semver.MustParse(minVersion))
}

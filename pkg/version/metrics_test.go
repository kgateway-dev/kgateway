package version

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

func TestBuildInfo(t *testing.T) {
	// The values depend on how the test binary was built (LDFLAGS, VCS stamping),
	// so the expected series is built from the same source the metric uses.
	info := version{Controller: UndefinedVersion, OS: runtime.GOOS, Arch: runtime.GOARCH}
	if ref != nil {
		info = *ref
	}
	expected := fmt.Sprintf(`
# HELP kgateway_build_info Build information of the running controller. The series always has value 1.
# TYPE kgateway_build_info gauge
kgateway_build_info{goarch=%q,goos=%q,goversion=%q,revision=%q,version=%q} 1
`, info.Arch, info.OS, runtime.Version(), info.Commit, info.Controller)

	err := testutil.GatherAndCompare(metrics.Registry(), strings.NewReader(expected), "kgateway_build_info")
	require.NoError(t, err, "kgateway_build_info should be registered at init")
}

package version

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func TestRecordBuildInfo(t *testing.T) {
	// The values depend on how the test binary was built (LDFLAGS, VCS stamping),
	// so the expected series is built from the same source the metric uses.
	info := current()
	activeSeries := fmt.Sprintf(`
# HELP kgateway_build_info Build information of the running controller. The series always has value 1.
# TYPE kgateway_build_info gauge
kgateway_build_info{goarch=%q,goos=%q,goversion=%q,revision=%q,version=%q} 1
`, info.Arch, info.OS, runtime.Version(), info.Commit, info.Controller)

	tests := []struct {
		name     string
		active   bool
		expected string
	}{
		{name: "metrics active", active: true, expected: activeSeries},
		{name: "metrics inactive", active: false, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wasActive := metrics.Active()
			metrics.SetActive(tt.active)
			buildInfo.Reset()
			t.Cleanup(func() {
				buildInfo.Reset()
				metrics.SetActive(wasActive)
			})

			RecordBuildInfo()

			err := metricstest.CollectAndCompare(buildInfo, strings.NewReader(tt.expected), "kgateway_build_info")
			require.NoError(t, err)
		})
	}
}

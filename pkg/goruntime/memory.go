package goruntime

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

const (
	// AutoMemoryLimitEnv configures the fraction of the container memory limit
	// used for GOMEMLIMIT.
	AutoMemoryLimitEnv = "AUTOMEMLIMIT"

	goMemoryLimitEnv = "GOMEMLIMIT"
	refreshInterval  = 30 * time.Second
)

var logger = logging.New("go-runtime")

// ConfigureMemoryLimit configures the Go runtime memory limit from the
// container's live cgroup memory limit when AUTOMEMLIMIT is set. The cgroup is
// polled so in-place container memory resizes are reflected without restarting
// the process. AUTOMEMLIMIT and GOMEMLIMIT follow memlimit.Set's environment
// variable semantics, including precedence for an explicit GOMEMLIMIT.
func ConfigureMemoryLimit(ctx context.Context) error {
	return configureMemoryLimit(ctx, memlimit.FromCgroup, refreshInterval)
}

func configureMemoryLimit(ctx context.Context, provider memlimit.Provider, refresh time.Duration) error {
	// Keep this behavior opt-in. The Helm chart sets AUTOMEMLIMIT when
	// goMemLimitPercent is configured; processes started outside the chart retain
	// the Go runtime's existing behavior unless the operator opts in explicitly.
	autoMemoryLimit, ok := os.LookupEnv(AutoMemoryLimitEnv)
	if !ok {
		return nil
	}
	// Match memlimit.Set's precedence exactly: an explicit GOMEMLIMIT wins, and
	// AUTOMEMLIMIT=off disables configuration without parsing a ratio.
	if goMemoryLimit, ok := os.LookupEnv(goMemoryLimitEnv); ok {
		logger.Info("GOMEMLIMIT is already set, skipping container-aware configuration", "value", goMemoryLimit)
		return nil
	}
	if autoMemoryLimit == "off" {
		logger.Info("AUTOMEMLIMIT is off, skipping container-aware configuration")
		return nil
	}
	if err := validateAutoMemoryLimit(autoMemoryLimit); err != nil {
		return err
	}

	limit, err := memlimit.Set(
		memlimit.WithProvider(provider),
		memlimit.WithRefreshInterval(ctx, refresh),
		memlimit.WithLogger(logger),
	)
	if err != nil {
		logger.Warn("continuing without an initial container-aware GOMEMLIMIT; the refresh loop will retry", "error", err)
		return nil
	}
	if limit == math.MaxInt64 {
		logger.Warn("container has no finite memory limit; AUTOMEMLIMIT will not constrain the Go runtime until a limit is available")
	}
	return nil
}

// validateAutoMemoryLimit keeps configuration errors fatal while allowing
// provider errors to be retried by automemlimit's refresh loop.
func validateAutoMemoryLimit(value string) error {
	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(ratio) || ratio <= 0 || ratio > 1 {
		return fmt.Errorf("AUTOMEMLIMIT=%q must be a ratio in the range (0.0,1.0]", value)
	}
	return nil
}

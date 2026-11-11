package goruntime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

const (
	// AutoMemoryLimitEnv configures the fraction of the container memory limit
	// used for GOMEMLIMIT.
	AutoMemoryLimitEnv = "AUTOMEMLIMIT"

	// AutoMemoryLimitFallbackEnv configures a fallback memory limit in bytes
	// used when AUTOMEMLIMIT is enabled and the cgroup reports no finite limit.
	AutoMemoryLimitFallbackEnv = "KGW_GOMEMLIMIT_FALLBACK"

	goMemoryLimitEnv = "GOMEMLIMIT"
	refreshInterval  = 30 * time.Second
)

var logger = logging.New("go-runtime")

// ConfigureMemoryLimit configures the Go runtime memory limit from the
// container's live cgroup memory limit when AUTOMEMLIMIT is set. The cgroup is
// polled so in-place container memory resizes are reflected without restarting
// the process. When KGW_GOMEMLIMIT_FALLBACK is set and the cgroup reports no
// finite limit (memlimit.ErrNoLimit), AUTOMEMLIMIT is applied to that fallback
// until a live cgroup limit becomes available. AUTOMEMLIMIT and GOMEMLIMIT
// follow memlimit.Set's environment variable semantics, including precedence
// for an explicit GOMEMLIMIT.
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
	fallback, hasFallback, err := parseAutoMemoryLimitFallback()
	if err != nil {
		return err
	}

	limit, err := memlimit.Set(
		memlimit.WithProvider(providerWithFallback(provider, fallback, hasFallback)),
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

func parseAutoMemoryLimitFallback() (uint64, bool, error) {
	raw, ok := os.LookupEnv(AutoMemoryLimitFallbackEnv)
	if !ok {
		return 0, false, nil
	}
	fallback, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || fallback == 0 || fallback > math.MaxInt64 {
		return 0, false, fmt.Errorf("%s=%q must be an integer between 1 and %d bytes", AutoMemoryLimitFallbackEnv, raw, int64(math.MaxInt64))
	}
	return fallback, true, nil
}

// providerWithFallback falls back to fallback only when the cgroup reports no
// finite limit; other errors still propagate so memlimit logs and retries them.
func providerWithFallback(provider memlimit.Provider, fallback uint64, hasFallback bool) memlimit.Provider {
	if !hasFallback {
		return provider
	}
	var loggedFallback atomic.Bool
	return func() (uint64, error) {
		limit, err := provider()
		if err == nil {
			loggedFallback.Store(false)
			return limit, nil
		}
		if errors.Is(err, memlimit.ErrNoLimit) {
			if loggedFallback.CompareAndSwap(false, true) {
				logger.Info(
					"container has no finite memory limit; applying fallback limit for AUTOMEMLIMIT",
					"fallback_bytes", fallback,
				)
			}
			return fallback, nil
		}
		return 0, err
	}
}

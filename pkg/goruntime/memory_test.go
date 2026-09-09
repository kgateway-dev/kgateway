package goruntime

import (
	"context"
	"errors"
	"math"
	"os"
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/stretchr/testify/require"
)

func TestConfigureMemoryLimitIsOptIn(t *testing.T) {
	unsetEnv(t, AutoMemoryLimitEnv)

	called := false
	err := configureMemoryLimit(context.Background(), func() (uint64, error) {
		called = true
		return 0, errors.New("provider should not be called")
	}, time.Millisecond)

	require.NoError(t, err)
	require.False(t, called)
}

func TestAutomemlimitEnvironmentContract(t *testing.T) {
	t.Run("ratio", func(t *testing.T) {
		unsetEnv(t, goMemoryLimitEnv)
		t.Setenv(AutoMemoryLimitEnv, "0.25")
		restoreMemoryLimit(t)

		limit, err := memlimit.Set(memlimit.WithProvider(memlimit.Limit(1_000)))

		require.NoError(t, err)
		require.Equal(t, int64(250), limit)
		require.Equal(t, int64(250), debug.SetMemoryLimit(-1))
	})

	t.Run("off", func(t *testing.T) {
		unsetEnv(t, goMemoryLimitEnv)
		t.Setenv(AutoMemoryLimitEnv, "off")
		restoreMemoryLimit(t)
		const initialLimit = int64(1 << 30)
		debug.SetMemoryLimit(initialLimit)

		called := false
		limit, err := memlimit.Set(memlimit.WithProvider(func() (uint64, error) {
			called = true
			return 1_000, nil
		}))

		require.NoError(t, err)
		require.Equal(t, initialLimit, limit)
		require.Equal(t, initialLimit, debug.SetMemoryLimit(-1))
		require.False(t, called)
	})
}

func TestConfigureMemoryLimitRefreshesFromProvider(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	t.Setenv(AutoMemoryLimitEnv, "0.5")
	restoreMemoryLimit(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		// automemlimit intentionally does not expose its refresh goroutine. Give
		// it time to observe cancellation before restoring the process-global
		// memory limit in the next cleanup.
		time.Sleep(10 * time.Millisecond)
	})

	var containerLimit atomic.Uint64
	containerLimit.Store(1_000)
	err := configureMemoryLimit(ctx, func() (uint64, error) {
		return containerLimit.Load(), nil
	}, 5*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, int64(500), debug.SetMemoryLimit(-1))

	containerLimit.Store(800)
	require.Eventually(t, func() bool {
		return debug.SetMemoryLimit(-1) == 400
	}, time.Second, time.Millisecond)
}

func TestConfigureMemoryLimitHonorsExplicitGoMemLimit(t *testing.T) {
	t.Setenv(AutoMemoryLimitEnv, "invalid")
	t.Setenv("GOMEMLIMIT", "256MiB")

	called := false
	err := configureMemoryLimit(context.Background(), func() (uint64, error) {
		called = true
		return 0, errors.New("provider should not be called")
	}, time.Millisecond)

	require.NoError(t, err)
	require.False(t, called)
}

func TestConfigureMemoryLimitRejectsInvalidRatio(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	t.Setenv(AutoMemoryLimitEnv, "invalid")
	restoreMemoryLimit(t)

	err := configureMemoryLimit(context.Background(), memlimit.Limit(1_000), time.Millisecond)
	require.ErrorContains(t, err, `AUTOMEMLIMIT="invalid" must be a ratio in the range (0.0,1.0]`)
}

func TestConfigureMemoryLimitRejectsOutOfRangeRatio(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	t.Setenv(AutoMemoryLimitEnv, "1.1")
	restoreMemoryLimit(t)

	err := configureMemoryLimit(context.Background(), memlimit.Limit(1_000), time.Millisecond)
	require.ErrorContains(t, err, `AUTOMEMLIMIT="1.1" must be a ratio in the range (0.0,1.0]`)
}

func TestConfigureMemoryLimitHonorsOff(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	t.Setenv(AutoMemoryLimitEnv, "off")
	restoreMemoryLimit(t)

	called := false
	err := configureMemoryLimit(context.Background(), func() (uint64, error) {
		called = true
		return 0, errors.New("provider should not be called")
	}, time.Millisecond)

	require.NoError(t, err)
	require.False(t, called)
}

func TestConfigureMemoryLimitWithoutContainerLimit(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	t.Setenv(AutoMemoryLimitEnv, "0.9")
	restoreMemoryLimit(t)

	err := configureMemoryLimit(context.Background(), func() (uint64, error) {
		return 0, memlimit.ErrNoLimit
	}, 0)

	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), debug.SetMemoryLimit(-1))
}

func TestConfigureMemoryLimitRecoversFromInitialProviderError(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	t.Setenv(AutoMemoryLimitEnv, "0.5")
	restoreMemoryLimit(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		time.Sleep(10 * time.Millisecond)
	})

	var calls atomic.Uint32
	err := configureMemoryLimit(ctx, func() (uint64, error) {
		if calls.Add(1) == 1 {
			return 0, errors.New("temporary cgroup read failure")
		}
		return 1_000, nil
	}, 5*time.Millisecond)

	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return debug.SetMemoryLimit(-1) == 500
	}, time.Second, time.Millisecond)
}

func restoreMemoryLimit(t *testing.T) {
	t.Helper()
	previous := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetMemoryLimit(previous)
	})
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	value, ok := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))
	t.Cleanup(func() {
		if ok {
			require.NoError(t, os.Setenv(name, value))
			return
		}
		require.NoError(t, os.Unsetenv(name))
	})
}
